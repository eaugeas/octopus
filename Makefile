GO=go
GOBUILD=go build
GOTEST=go test
GOBENCH=go test -test.bench Benchmark
GOCOVER=go tool cover
GOVET=go vet

all: build

build:
	$(GOBUILD) ./...

test:
	$(GOTEST) ./...

bench:
	$(GOTEST) ./... -test.bench Benchmark

lint:
	$(GOVET) ./...

coverage.out: test
	$(GOTEST) ./... -coverprofile=coverage.out

coverage-html: coverage.out
	$(GOCOVER) -html coverage.out
