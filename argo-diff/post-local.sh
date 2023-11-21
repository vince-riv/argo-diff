#!/usr/bin/env bash

curl -v -X POST \
    --header @temp/curl-headers.txt \
	--data @temp/curl-payload.json \
	http://127.0.0.1:8080/webhook
