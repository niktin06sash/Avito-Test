.PHONY: integration-test unit-test lint start stop migrate-up migrate-down logs status up down clean

integration-test:
		go test -v ./tests/...

unit-test:
		mkdir -p unit-test
		go test -v ./internal/service... -coverprofile=unit-test/coverage.out -coverpkg=./internal/...
		go tool cover -html=unit-test/coverage.out -o unit-test/coverage.html

lint:
		golangci-lint run

start:
		docker-compose up --build -d

stop:
		docker-compose stop

down:
		docker-compose down

clean:
		docker-compose down -v
		rm -rf unit-test/coverage.out unit-test/coverage.html

migrate-up:
		migrate -path ./migrations -database "postgres://postgres:postgres@localhost:5433/booking?sslmode=disable" up

migrate-down:
		migrate -path ./migrations -database "postgres://postgres:postgres@localhost:5433/booking?sslmode=disable" down

logs:
		docker-compose logs -f

status:
		docker-compose ps

up: lint unit-test integration-test start migrate-up status