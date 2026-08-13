# Keep in sync with the lint job in .github/workflows/ci.yml.
GOLANGCI_LINT_VERSION := v2.12.2

.PHONY: all build test test-simd race check fmt fmt-check vet lint bench bench-vs-art bench-smoke fuzz tools

all: check test-simd lint

build:
	go build ./
	GOEXPERIMENT=simd go build ./

test:
	go test -count=1 ./

test-simd:
	GOEXPERIMENT=simd go test -count=1 ./

race:
	GOEXPERIMENT=simd go test -race -count=1 ./

# check is the CI test-job recipe. It honors GOEXPERIMENT from the
# environment: `make check` tests the scalar build,
# `GOEXPERIMENT=simd make check` the SIMD build.
check: fmt-check vet
	go test -race -count=1 ./

fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

fmt:
	gofmt -w .

vet:
	go vet ./

lint:
	golangci-lint run ./

bench:
	GOEXPERIMENT=simd go test -run xxx -bench . -benchmem ./

bench-vs-art:
	cd benchmark && GOEXPERIMENT=simd go test -run xxx -bench . -benchmem .

# bench-smoke is the CI recipe: proves the benchmark module compiles and runs.
bench-smoke:
	cd benchmark && GOEXPERIMENT=simd go test -run xxx -bench 'BenchmarkTreeSearch/HSK' -benchtime 1x .

fuzz:
	go test -run '^$$' -fuzz FuzzTreeOps -fuzztime 30s ./
	go test -run '^$$' -fuzz FuzzKeyEncoding -fuzztime 30s ./

tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
