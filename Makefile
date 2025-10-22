# Copyright 2017 Heptio Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

TARGET ?= eventrouter
COVERAGE_FILE ?= coverage.out
COVERAGE_HTML_FILE ?= coverage.html

all: vet test

.PHONY: build
build:
	go build -o ${TARGET}

.PHONY: test
test:
	go test ./... -v -timeout 60s

.PHONY: test-short
test-short:
	go test ./... -v -timeout 30s -short

.PHONY: test-coverage
test-coverage:
	go test ./... -v -timeout 60s -coverprofile=${COVERAGE_FILE} -covermode=atomic

.PHONY: test-coverage-html
test-coverage-html: test-coverage
	go tool cover -html=${COVERAGE_FILE} -o ${COVERAGE_HTML_FILE}
	@echo "Coverage report generated: ${COVERAGE_HTML_FILE}"

.PHONY: test-coverage-report
test-coverage-report: test-coverage
	go tool cover -func=${COVERAGE_FILE}

.PHONY: benchmark
benchmark:
	go test ./... -bench=. -benchmem -timeout 300s

.PHONY: benchmark-verbose
benchmark-verbose:
	go test ./... -bench=. -benchmem -timeout 300s -v

.PHONY: benchmark-cpu
benchmark-cpu:
	go test ./... -bench=. -benchmem -timeout 300s -cpuprofile=cpu.prof

.PHONY: benchmark-mem
benchmark-mem:
	go test ./... -bench=. -benchmem -timeout 300s -memprofile=mem.prof

.PHONY: integration-test
integration-test:
	go test ./... -v -timeout 120s -run="TestIntegration"

.PHONY: unit-test
unit-test:
	go test ./... -v -timeout 60s -run="^Test[^I]" # Exclude integration tests

.PHONY: vet
vet:
	go vet ./...

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: clean
clean:
	rm -f ${TARGET} ${COVERAGE_FILE} ${COVERAGE_HTML_FILE} *.prof

.PHONY: test-all
test-all: vet fmt test-coverage benchmark

.PHONY: ci
ci: vet test-coverage
	@echo "CI pipeline completed successfully"

.PHONY: ci-quick
ci-quick: vet test-short
	@echo "Quick CI checks completed"

.PHONY: test-race
test-race:
	go test ./... -race -short

.PHONY: security-scan
security-scan:
	@echo "Running security checks..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not installed, skipping security scan"; \
	fi

.PHONY: mod-verify
mod-verify:
	go mod verify
	go mod tidy
	@if [ -n "$$(git diff --name-only go.mod go.sum)" ]; then \
		echo "go.mod or go.sum needs updating:"; \
		git diff go.mod go.sum; \
		exit 1; \
	fi

.PHONY: dev
dev: fmt vet test-short
	@echo "Development checks completed"

.PHONY: help
help:
	@echo "EventRouter Makefile Commands:"
	@echo ""
	@echo "Build Commands:"
	@echo "  build              Build the eventrouter binary"
	@echo ""
	@echo "Test Commands:"
	@echo "  test               Run all tests with verbose output"
	@echo "  test-short         Run tests with -short flag (faster)"
	@echo "  test-coverage      Run tests with coverage analysis"
	@echo "  test-coverage-html Generate HTML coverage report"
	@echo "  test-coverage-report Show coverage report in terminal"
	@echo "  unit-test          Run only unit tests (exclude integration)"
	@echo "  integration-test   Run only integration tests"
	@echo ""
	@echo "Benchmark Commands:"
	@echo "  benchmark          Run all benchmarks"
	@echo "  benchmark-verbose  Run benchmarks with verbose output"
	@echo "  benchmark-cpu      Run benchmarks with CPU profiling"
	@echo "  benchmark-mem      Run benchmarks with memory profiling"
	@echo ""
	@echo "Quality Commands:"
	@echo "  vet                Run go vet"
	@echo "  fmt                Run go fmt"
	@echo "  lint               Run golangci-lint (requires installation)"
	@echo "  test-race          Run tests with race condition detection"
	@echo "  security-scan      Run security analysis (gosec)"
	@echo "  mod-verify         Verify and tidy go.mod"
	@echo ""
	@echo "Aggregate Commands:"
	@echo "  all                Run vet and test (default)"
	@echo "  test-all           Run vet, fmt, coverage tests, and benchmarks"
	@echo "  ci                 Run CI pipeline (vet + coverage)"
	@echo "  ci-quick           Run quick CI checks (vet + short tests)"
	@echo "  dev                Run development checks (fmt + vet + short tests)"
	@echo ""
	@echo "Utility Commands:"
	@echo "  clean              Remove built binaries and coverage files"
	@echo "  help               Show this help message"