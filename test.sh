#!/usr/bin/env bash

go build
protoc -I=. --plugin=protoc-gen-bean --bean_out=vopackage=vo,cvtpackage=protobuf.converter:./temp common.proto hello.proto