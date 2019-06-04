#!/bin/bash
docker build -t sip-ping-builder .
docker run --rm -v "$PWD":/go/src/sip-ping -w /go/src/sip-ping sip-ping-builder go get -v && go build -v -ldflags="-s -w"
