@ECHO OFF

SETLOCAL ENABLEEXTENSIONS
SET me=%~n0
SET parent=%~dp0

ECHO ON
go build
move protoc-gen-bean.exe protoc-gen-bean
protoc.exe -I=. --plugin=protoc-gen-bean --bean_out temp hello.proto
