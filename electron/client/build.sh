#!/bin/bash

npm run build:linux && \
	docker build -t crawler_client . && \
	npm run clean
