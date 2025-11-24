# How to Run Tests

## Quick Test Run

```bash
# From project root
cd /home/dylan/repos/openswiss

# Run all tests
go test ./... -v

# Run tests for specific packages
go test ./internal/storage -v
go test ./internal/auth -v  
go test ./internal/handlers -v
```

## Test Files Created

1. **`internal/storage/tournament_test.go`** - Tests storage layer
2. **`internal/auth/auth_test.go`** - Tests authentication
3. **`internal/handlers/handlers_test.go`** - Tests HTTP handlers
4. **`internal/handlers/integration_test.go`** - End-to-end workflow tests

## Current Test Status

- ✅ Storage tests: Most passing
- ✅ Auth tests: All passing  
- ⚠️ Handler tests: Some fail due to template path issues (need to run from project root)

## Running Individual Tests

```bash
# Test player registration
go test ./internal/storage -v -run TestAddPendingPlayer

# Test accepting players
go test ./internal/storage -v -run TestAcceptPlayer

# Test full workflow
go test ./internal/storage -v -run TestTournamentWorkflow

# Test authentication
go test ./internal/auth -v

# Test full integration
go test ./internal/handlers -v -run TestFullTournamentFlow
```


