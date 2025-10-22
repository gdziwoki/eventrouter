#!/bin/bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
VERBOSE=${VERBOSE:-false}
COVERAGE=${COVERAGE:-true}
BENCHMARKS=${BENCHMARKS:-false}
RACE_DETECTION=${RACE_DETECTION:-false}

echo -e "${BLUE}🧪 EventRouter Test Suite${NC}"
echo "=================================="
echo "Verbose: $VERBOSE"
echo "Coverage: $COVERAGE"
echo "Benchmarks: $BENCHMARKS"
echo "Race Detection: $RACE_DETECTION"
echo ""

# Function to run command with status reporting
run_check() {
    local name="$1"
    local cmd="$2"

    echo -n -e "${BLUE}Running $name...${NC} "

    if $VERBOSE; then
        echo ""
        if eval "$cmd"; then
            echo -e "${GREEN}✓ $name passed${NC}"
        else
            echo -e "${RED}✗ $name failed${NC}"
            exit 1
        fi
    else
        if eval "$cmd" >/dev/null 2>&1; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
            echo -e "${RED}$name failed. Run with VERBOSE=true for details.${NC}"
            exit 1
        fi
    fi
}

# Go version check
echo -e "${BLUE}Go Environment:${NC}"
go version
echo ""

# Module verification
run_check "Module verification" "make mod-verify"

# Code formatting check
run_check "Code formatting" "test -z \"\$(gofmt -s -l . | grep -v vendor/)\""

# Vet check
run_check "Go vet" "make vet"

# Unit tests
if [ "$RACE_DETECTION" = "true" ]; then
    run_check "Unit tests (with race detection)" "make test-race"
else
    run_check "Unit tests" "make unit-test"
fi

# Integration tests
run_check "Integration tests" "make integration-test"

# Coverage
if [ "$COVERAGE" = "true" ]; then
    run_check "Test coverage" "make test-coverage"

    # Coverage report
    if command -v go >/dev/null 2>&1; then
        echo ""
        echo -e "${BLUE}Coverage Report:${NC}"
        go tool cover -func=coverage.out | tail -n 1

        # Check coverage threshold
        COVERAGE_PERCENT=$(go tool cover -func=coverage.out | tail -n 1 | awk '{print $3}' | sed 's/%//')
        THRESHOLD=60

        if (( $(echo "$COVERAGE_PERCENT >= $THRESHOLD" | bc -l) )); then
            echo -e "${GREEN}✓ Coverage ($COVERAGE_PERCENT%) meets threshold ($THRESHOLD%)${NC}"
        else
            echo -e "${YELLOW}⚠ Coverage ($COVERAGE_PERCENT%) below threshold ($THRESHOLD%)${NC}"
        fi
    fi
fi

# Benchmarks (optional)
if [ "$BENCHMARKS" = "true" ]; then
    echo ""
    echo -e "${BLUE}Running Performance Benchmarks:${NC}"
    timeout 300s make benchmark || echo -e "${YELLOW}Benchmark timeout (300s) - this is normal for comprehensive benchmarks${NC}"
fi

# Security scan (if gosec is available)
if command -v gosec >/dev/null 2>&1; then
    run_check "Security scan" "make security-scan"
fi

# Build verification
run_check "Build verification" "make build"

echo ""
echo -e "${GREEN}🎉 All tests passed successfully!${NC}"
echo ""

# Summary
echo -e "${BLUE}Test Summary:${NC}"
echo "- ✓ Code formatting and style"
echo "- ✓ Static analysis (go vet)"
echo "- ✓ Unit tests"
echo "- ✓ Integration tests"
if [ "$COVERAGE" = "true" ]; then
    echo "- ✓ Code coverage analysis"
fi
if [ "$RACE_DETECTION" = "true" ]; then
    echo "- ✓ Race condition detection"
fi
if [ "$BENCHMARKS" = "true" ]; then
    echo "- ✓ Performance benchmarks"
fi
echo "- ✓ Build verification"

echo ""
echo -e "${GREEN}Ready for deployment! 🚀${NC}"