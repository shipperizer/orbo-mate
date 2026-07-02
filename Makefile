.PHONY: build test test-short vet fmt mock

build:
	go build -v ./...

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
