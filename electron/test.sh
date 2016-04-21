#!/bin/bash

ID=$(docker run -p 4222:4222 -d nats)
[[ "$?" != "0" ]] && exit 1

(cd client && env CONN_MODE=nats npm run test &> /dev/null &)
go test -v
docker stop "$ID"
docker rm -f -v "$ID"
