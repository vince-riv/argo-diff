#!/usr/bin/env bash

curl -v -X POST \
    --header @temp/curl-comment-headers.txt \
	--data @temp/curl-comment-payload.json \
	http://127.0.0.1:8080/webhook
