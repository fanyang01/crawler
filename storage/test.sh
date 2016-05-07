#!/bin/bash

ID=$(docker run -p 5432:5432 -e POSTGRES_DB=test -d postgres)
if [[ "$?" = "0" ]]; then 
	sleep 2
	go test -v -cover -tags 'integration'
	docker stop "$ID"
	docker rm -f -v "$ID"
fi
