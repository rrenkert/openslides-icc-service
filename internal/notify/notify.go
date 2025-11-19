package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/OpenSlides/openslides-icc-service/internal/iccerror"
	"github.com/OpenSlides/openslides-icc-service/internal/icclog"
	"github.com/ostcar/topic"
)

// Backend stores the notify messages.
type Backend interface {
	// NotifyPublish saves a valid notify message.
	NotifyPublish([]byte) error

	// NotifyReceive is a blocking function that receives the messages.
	//
	// The first call returnes the first notify message, the next call the
	// second an so on. If there are no more messages to read, the function
	// blocks until there is or the context ist canceled.
	//
	// It is expected, that only one goroutine is calling this function. The
	// Backend keeps track what the last send message was.
	NotifyReceive(ctx context.Context) (message []byte, err error)
}

// Notify holds the state of the service.
type Notify struct {
	backend Backend
	cIDGen  cIDGen
	topic   *topic.Topic[string]
}

// New returns an initialized state of the notify service.
//
// The New function is not blocking. The context is used to stop a goroutine
// that is started by this function.
func New(b Backend) (*Notify, func(context.Context, func(error))) {
	notify := Notify{
		backend: b,
		topic:   topic.New[string](),
	}

	background := func(ctx context.Context, errHandler func(error)) {
		go notify.listen(ctx, errHandler)
	}

	return &notify, background
}

// listen waits for Notify messages from the backend and saves them into the
// topic.
func (n *Notify) listen(ctx context.Context, errhandler func(error)) {
	if errhandler == nil {
		errhandler = func(error) {}
	}

	for {
		m, err := n.backend.NotifyReceive(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}

			errhandler(fmt.Errorf("receicing data from backend: %w", err))
			time.Sleep(5 * time.Second)
			continue
		}

		icclog.Debug("Found notify message: `%s`", m)
		n.topic.Publish(string(m))
	}
}

// NextMessage is a function that can be called to get the next message.
type NextMessage func(context.Context) (OutMessage, error)

// Receive returns an individuel channel id and a channel to receive messages
// from.
func (n *Notify) Receive(meetingID, uid int) (cid string, nm NextMessage) {
	channelID := n.cIDGen.generate(uid)

	mp := messageProvider{
		tid:       n.topic.LastID(),
		uid:       uid,
		meetingID: meetingID,
		channelID: channelID,
		topic:     n.topic,
	}

	return channelID.String(), mp.Next
}

// Publish reads and saves the notify event from the given reader.
func (n *Notify) Publish(r io.Reader, uid int) error {
	var message Message
	if err := json.NewDecoder(r).Decode(&message); err != nil {
		return iccerror.NewMessageError(iccerror.ErrInvalid, "invalid json: %v", err)
	}

	if err := validateMessage(message, uid); err != nil {
		return fmt.Errorf("validate message: %w", err)
	}

	bs, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal notify message: %v", err)
	}

	icclog.Debug("Saving notify message: `%s`", bs)
	if err := n.backend.NotifyPublish(bs); err != nil {
		return fmt.Errorf("saving message in backend: %w", err)
	}

	return nil
}

func validateMessage(message Message, userID int) error {
	if message.ChannelID.uid() != userID {
		return iccerror.NewMessageError(iccerror.ErrInvalid, "invalid channel id `%s`", message.ChannelID)
	}

	if message.Name == "" {
		return iccerror.NewMessageError(iccerror.ErrInvalid, "notify message does not have required field `name`")
	}

	return nil
}

// Message is a message from the one client to all/some others.
type Message struct {
	ChannelID  channelID       `json:"channel_id"`
	ToMeeting  int             `json:"to_meeting,omitempty"`
	ToUsers    []int           `json:"to_users,omitempty"`
	ToChannels []string        `json:"to_channels,omitempty"`
	Name       string          `json:"name"`
	Message    json.RawMessage `json:"message"`
}

func (m Message) forMe(meetingID, uid int, cID channelID) bool {
	if m.ToMeeting != 0 && m.ToMeeting == meetingID {
		return true
	}

	if slices.Contains(m.ToUsers, uid) {
		return true
	}

	if slices.Contains(m.ToChannels, cID.String()) {
		return true
	}

	return false
}

// OutMessage is a message that is going out of the service.
type OutMessage struct {
	SenderUserID    int             `json:"sender_user_id"`
	SenderChannelID string          `json:"sender_channel_id"`
	Name            string          `json:"name"`
	Message         json.RawMessage `json:"message"`
}

// messageProvider returns messages by calling Next().
type messageProvider struct {
	tid       uint64
	uid       int
	meetingID int
	channelID channelID

	topic      *topic.Topic[string]
	messageBuf []string
}

// Next returns the next message. Can be called many times.
func (mp *messageProvider) Next(ctx context.Context) (OutMessage, error) {
	var message Message

	for {
		if len(mp.messageBuf) == 0 {
			tid, messages, err := mp.topic.ReceiveSince(ctx, mp.tid)
			if err != nil {
				return OutMessage{}, fmt.Errorf("fetching message from topic: %w", err)
			}

			mp.tid = tid
			mp.messageBuf = messages
		}

		m := mp.messageBuf[0]
		mp.messageBuf = mp.messageBuf[1:]

		if err := json.Unmarshal([]byte(m), &message); err != nil {
			return OutMessage{}, fmt.Errorf("decoding message: %w", err)
		}

		if message.forMe(mp.meetingID, mp.uid, mp.channelID) {
			break
		}
	}

	out := OutMessage{
		message.ChannelID.uid(),
		message.ChannelID.String(),
		message.Name,
		message.Message,
	}

	return out, nil
}
