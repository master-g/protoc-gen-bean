@ECHO OFF

SETLOCAL ENABLEEXTENSIONS
SET me=%~n0
SET parent=%~dp0

ECHO ON
protoc.exe -I=. --plugin=protoc-gen-bean --bean_out=vopackage=vo,cvtpackage=protobuf.converter:temp common.proto hello.proto
