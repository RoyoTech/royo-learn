.PHONY: fmt tidy-check verify test vet build quality

fmt:
	go fmt ./...

tidy-check:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

verify:
	go mod verify

test:
	go test ./...

vet:
	go vet ./...

build:
	go build ./cmd/royo-learn

quality: fmt tidy-check verify test vet build
