#!/usr/bin/env bash

go build
protoc -I=./test --plugin=protoc-gen-bean --bean_out=vopackage=vo,cvtpackage=protobuf.converter:./test ./test/common.proto ./test/hello.proto