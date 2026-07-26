.PHONY: build test lint

build:
	go build -buildvcs=false ./...

test:
	go test -buildvcs=false ./...

lint:
	go vet -buildvcs=false ./...
