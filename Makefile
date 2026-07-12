.PHONY: fmt tidy-check verify test vet build quality build-all install clean

BINARY_NAME := royo-learn
ifeq ($(OS),Windows_NT)
	BINARY_NAME := royo-learn.exe
endif

fmt:
	go fmt ./... && git diff --exit-code

tidy-check:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

verify:
	go mod verify

test:
	go test -race ./...

vet:
	go vet ./...

build:
	go build ./cmd/royo-learn

build-all: ## Cross-compile for all platforms
	GOOS=windows GOARCH=amd64 go build -o dist/royo-learn-windows-amd64.exe ./cmd/royo-learn
	GOOS=linux GOARCH=amd64 go build -o dist/royo-learn-linux-amd64 ./cmd/royo-learn
	GOOS=linux GOARCH=arm64 go build -o dist/royo-learn-linux-arm64 ./cmd/royo-learn
	GOOS=darwin GOARCH=amd64 go build -o dist/royo-learn-darwin-amd64 ./cmd/royo-learn
	GOOS=darwin GOARCH=arm64 go build -o dist/royo-learn-darwin-arm64 ./cmd/royo-learn

install: build ## Install locally
	cp $(BINARY_NAME) $(shell go env GOPATH)/bin/royo-learn 2>/dev/null || true

clean: ## Remove build artifacts
	rm -rf dist/ royo-learn royo-learn.exe

quality: fmt tidy-check verify test vet build
