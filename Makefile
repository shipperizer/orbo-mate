.PHONY: build build-arm64 test test-short vet fmt mock

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o orbo-mate main.go

build-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -v -o orbo-mate-arm64 main.go

test:
	go test -race ./...

test-short:
	go test -race -short ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

mock:
	go run main.go mock-webhook --pr-url $(url)
