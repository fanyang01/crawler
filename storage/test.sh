#!/bin/bash

ID=$(docker run -p 5432:5432 -e POSTGRES_DB=test -d postgres)
if [[ "$?" = "0" ]]; then 
	go test -v -race -cover -tags 'integration'
	docker stop "$ID"
	docker rm -f -v "$ID"
fi
