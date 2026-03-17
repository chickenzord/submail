.PHONY: build test coverage lint run clean

BIN := submail
COVERAGE_FILE := coverage.txt
CONFIG ?= config.yaml

## build: compile the binary
build:
	go build -o $(BIN) ./cmd/submail

## test: run tests (matches CI)
test:
	go test -v -coverprofile=$(COVERAGE_FILE) ./...

## coverage: show coverage summary after running tests
coverage: test
	go tool cover -func=$(COVERAGE_FILE)

## coverage-html: open coverage report in browser
coverage-html: test
	go tool cover -html=$(COVERAGE_FILE)

## lint: run go vet
lint:
	go vet ./...

## run: run the server (CONFIG defaults to config.yaml)
run:
	go run ./cmd/submail server --config $(CONFIG)

## tidy: tidy go modules
tidy:
	go mod tidy

## clean: remove build artifacts
clean:
	rm -f $(BIN) $(COVERAGE_FILE)
