GO=go
GOBUILD=go build
GOTEST=go test
GOBENCH=go test -test.bench Benchmark
GOCOVER=go tool cover
GOVET=go tool vet

all: build

build:
	$(GOBUILD) github.com/tlblanc/octopus/container/tree
	$(GOBUILD) github.com/tlblanc/octopus/container/interval
	$(GOBUILD) github.com/tlblanc/octopus/concurrent

test:
	$(GOTEST) github.com/tlblanc/octopus/container/tree
	$(GOTEST) github.com/tlblanc/octopus/container/interval
	$(GOTEST) github.com/tlblanc/octopus/concurrent

bench:
	$(GOTEST) github.com/tlblanc/octopus/container/tree -test.bench Benchmark
	$(GOTEST) github.com/tlblanc/octopus/container/interval -test.bench Benchmark
	$(GOTEST) github.com/tlblanc/octopus/concurrent -test.bench Benchmark

lint:
	$(GOVET) container/tree
	$(GOVET) container/interval
	$(GOVET) container/concurrent

coverage.out: test
	$(GOTEST) ./... -coverprofile=coverage.out

coverage-html: coverage.out
	$(GOCOVER) -html coverage.out
