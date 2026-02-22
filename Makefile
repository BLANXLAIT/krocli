.PHONY: build lint clean

build:
	go build -o bin/krocli ./cmd/krocli

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/
