.PHONY: fmt tidy-check verify test vet build quality build-windows install clean

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

build-windows: ## Cross-compile for Windows (amd64 + arm64). Windows-only project.
	GOOS=windows GOARCH=amd64 go build -o dist/royo-learn-windows-amd64.exe ./cmd/royo-learn
	GOOS=windows GOARCH=arm64 go build -o dist/royo-learn-windows-arm64.exe ./cmd/royo-learn

install: build ## Install locally
	cp $(BINARY_NAME) $(shell go env GOPATH)/bin/royo-learn 2>/dev/null || true

clean: ## Remove build artifacts
	rm -rf dist/ royo-learn royo-learn.exe

quality: fmt tidy-check verify test vet build
