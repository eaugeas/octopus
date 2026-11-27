GO=go
GOBUILD=go build
GOTEST=go test
GOBENCH=go test -test.bench Benchmark
GOCOVER=go tool cover
GOVET=go vet
GOFMT=gofmt

all: build

build:
	$(GOBUILD) ./...

test:
	$(GOTEST) -race ./...

bench:
	$(GOTEST) ./... -test.bench Benchmark

lint:
	$(GOVET) ./...

# fmt-check fails if any file is not formatted with gofmt
fmt-check:
	@test -z "$$($(GOFMT) -l .)" || { echo "files not formatted with gofmt:"; $(GOFMT) -l .; exit 1; }

# ci runs the same checks as .github/workflows/ci.yml
ci: fmt-check lint
	$(GOTEST) -race -cover ./...

coverage.out: test
	$(GOTEST) ./... -coverprofile=coverage.out

coverage-html: coverage.out
	$(GOCOVER) -html coverage.out

.PHONY: all build test bench lint fmt-check ci coverage-html
