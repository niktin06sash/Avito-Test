.PHONY: unit-test lint

unit-test:
	go test -v ./internal/service... -coverprofile=unit-test/coverage.out -coverpkg=./internal/...
	go tool cover -html=unit-test/coverage.out -o unit-test/coverage.html
lint:
	golangci-lint run