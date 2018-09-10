#!/usr/bin/env bash

protoc -I=. --plugin=protoc-gen-bean --bean_out=vopackage=vo,cvtpackage=protobuf.converter:. common.proto hello.proto