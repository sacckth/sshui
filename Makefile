.PHONY: build test dist clean

BINARY=sshui

build:
	go build -o $(BINARY) ./cmd/sshui/

test:
	go test ./...

dist:
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64 ./cmd/sshui/
	GOOS=linux GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64 ./cmd/sshui/

clean:
	rm -rf dist $(BINARY)
