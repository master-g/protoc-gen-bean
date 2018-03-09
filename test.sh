#!/usr/bin/env bash

go build
protoc -I=. --plugin=protoc-gen-bean --bean_out ./temp hello.proto