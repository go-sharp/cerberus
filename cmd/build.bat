@echo off

echo Building 32-bit cerberus binaries...
set GOARCH=386
go build -tags forceposix -o cerberus_32.exe .

echo Building 64-bit cerberus binaries...
set GOARCH=amd64
go build -tags forceposix -o cerberus_64.exe .