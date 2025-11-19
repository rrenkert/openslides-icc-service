# OpenSlides-ICC

The ICC Service is part of the OpenSlides environment. Clients can connect to it
and communicate with eachother.

IMPORTANT: The data are sent via an open http-connection. All browsers limit the
amount of open http1.1 connections to a domain. For this service to work, the
browser has to connect to the service with http2 and therefore needs https.


## Start

The service needs some secrets to run. You can create them with:

```
mkdir secrets
printf "password" > secrets/postgres_password
printf "my_token_key" > secrets/auth_token_key
printf "my_cookie_key" > secrets/auth_cookie_key
```

It also needs a running postgres and redis instance. You can start one with:

```
docker run  --network host -e POSTGRES_PASSWORD=password -e POSTGRES_USER=openslides -e POSTGRES_DB=openslides postgres:11
```

and

```
docker run --network host redis
```


### With Golang

```
export SECRETS_PATH=secrets
go build ./cmd/icc
./icc
```

### With Docker

The docker build uses the auth token. Either configure it to use the fake
services (see environment variables below) or make sure the service inside the
docker container can connect to redis and postgres. For example with
the docker argument --network host. The auth-secrets have to given as a file.

```
docker build . --tag openslides-icc
printf "my_token_key" > auth_token_key
printf "my_cookie_key" > auth_cookie_key
docker run --network host -v $PWD/auth_token_key:/run/secrets/auth_token_key -v $PWD/auth_cookie_key:/run/secrets/auth_cookie_key openslides-icc
```

## Test

### With Golang

```
go test ./...
```


## Examples

Curl needs the flag `-N / --no-buffer` or it can happen, that the output is not
printed immediately.


### Notify

To listen to messages, you can use this command:

```
curl -N localhot:9007/system/icc/notify?meeting_id=5
```

The meeting_id query argument is optional.

The output has the [json lines](https://jsonlines.org/) format.

The first line returns an individual channel-id. It has to be used later so
publish messages:

```
{"channel_id": "QRboMVjb:1:0"}
```

Each other other line is one notify message. It has the following format:

```
{"sender_user_id":1,"sender_channel_id":"8NWRQy18:1:0","name":"my message title","message":"my message"}
```

To publish a message, you can use the following request:

```
curl localhost:9007/system/icc/notify/publish -d '{
  "channel_id": "STRING_SEE_ABOVE",
  "to_meeting": 5,
  "to_users": [3,4],
  "to_channels": "some:valid:channel_id",
  "name": "my message title",
  "message": {"any":"valid","json":"data"}
}'
```

The example message would be received by all users that are in meeting 5, and to
all connections of the user 3 and 4 and the connection with the channel id
"some:valid:channel_id".

Only one of the to_* fields is required. All other fields are required.


### Applause

The applause service needs a running datastore-reader. For testing, you can use
the [fake
datastore](https://github.com/OpenSlides/openslides-autoupdate-service/tree/main/cmd/datastore)
from the autoupdate-service repo.

To listen to messages, you can use this command:

```
curl -N localhost:9007/system/icc/applause?meeting_id=1
```

The meeting_id argument is required.

The returned messages have the format:

```
{"level":5,"present_users":25}
```

To send applause, use:

```
curl localhost:9007/system/icc/applause/send?meeting_id=1
```

The argument meeting_id is required.


## Configuration

The service is configurated with environment variables. See [all environment
varialbes](environment.md).
