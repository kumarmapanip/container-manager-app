.PHONY: all linux darwin windows

all: linux darwin windows

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o containerctl-linux cmd/main.go

darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o containerctl-darwin cmd/main.go

windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o containerctl.exe cmd/main.go
