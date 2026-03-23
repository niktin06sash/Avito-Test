.PHONY: unit-test lint start stop migrate-up migrate-down logs status

unit-test:
	go test -v ./internal/service... -coverprofile=unit-test/coverage.out -coverpkg=./internal/...
	go tool cover -html=unit-test/coverage.out -o unit-test/coverage.html
lint:
	golangci-lint run
start:
	docker-compose up --build
stop:
	docker-compose stop
migrate-up:
	migrate -path ./migrations -database "postgres://postgres:postgres@localhost:5433/booking?sslmode=disable" up
migrate-down:
	migrate -path ./migrations -database "postgres://postgres:postgres@localhost:5433/booking?sslmode=disable" down
logs:
	docker-compose logs -f
status:
	docker-compose ps