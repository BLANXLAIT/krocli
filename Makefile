.PHONY: build lint test clean

build:
	go build -o bin/krocli ./cmd/krocli

lint:
	golangci-lint run ./...

test:
	go test -race ./...

clean:
	rm -rf bin/
