@echo off

gofmt -s -w .
gofmt -s -w ../
go run build.go