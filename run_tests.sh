#!/bin/bash
# run_tests.sh

echo "Running all tests for ProjectorBackend"
echo "======================================"

# Unit tests
echo -e "\n📦 Running unit tests..."
go test ./pkg/... -v -cover

# Service tests
echo -e "\n🔧 Testing service-registry..."
go test ./services/service-registry/... -v -cover

echo -e "\n🔐 Testing identity-provider..."
go test ./services/identity-provider/... -v -cover

echo -e "\n📧 Testing mail-service..."
go test ./services/mail-service/... -v -cover

echo -e "\n🚪 Testing api-gateway..."
go test ./services/api-gateway/... -v -cover

# Integration tests (if not in short mode)
if [ "$1" != "--short" ]; then
    echo -e "\n🔗 Running integration tests..."
    go test ./services/identity-provider/test/integration/... -v -tags=integration
fi

# Benchmarks
echo -e "\n⚡ Running benchmarks..."
go test ./services/identity-provider/test/benchmark/... -bench=. -benchmem

# Generate coverage report
echo -e "\n📊 Generating coverage report..."
go test ./pkg/... ./services/... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

echo -e "\n✅ All tests completed!"