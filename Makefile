.PHONY: build run run-dev clean test test-coverage deps hashpw help

.DEFAULT_GOAL := help

APP := u
CMD := ./cmd/u

build: ## Build the binary
	go build -o $(APP) $(CMD)

help: ## Show this help message
	@echo ""
	@echo "██╗░░░██╗██████╗░██╗░░░░░  ░██████╗██╗░░██╗░█████╗░██████╗░████████╗███████╗███╗░░██╗███████╗██████╗░"
	@echo "██║░░░██║██╔══██╗██║░░░░░  ██╔════╝██║░░██║██╔══██╗██╔══██╗╚══██╔══╝██╔════╝████╗░██║██╔════╝██╔══██╗"
	@echo "██║░░░██║██████╔╝██║░░░░░  ╚█████╗░███████║██║░░██║██████╔╝░░░██║░░░█████╗░░██╔██╗██║█████╗░░██████╔╝"
	@echo "██║░░░██║██╔══██╗██║░░░░░  ░╚═══██╗██╔══██║██║░░██║██╔══██╗░░░██║░░░██╔══╝░░██║╚████║██╔══╝░░██╔══██╗"
	@echo "╚██████╔╝██║░░██║███████╗  ██████╔╝██║░░██║╚█████╔╝██║░░██║░░░██║░░░███████╗██║░╚███║███████╗██║░░██║"
	@echo "░╚═════╝░╚═╝░░╚═╝╚══════╝  ╚═════╝░╚═╝░░╚═╝░╚════╝░╚═╝░░╚═╝░░░╚═╝░░░╚══════╝╚═╝░░╚══╝╚══════╝╚═╝░░╚═╝"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run: build ## Build and run (PASSWORD=pass make run)
	U_ADMIN_PASSWORD=$${PASSWORD:?PASSWORD is required, e.g.: make run PASSWORD=yourpass} ./$(APP) data/config.yaml

run-dev: build ## Build and run with dev password (PASSWORD=dev by default)
	U_ADMIN_PASSWORD=$${PASSWORD:-dev} ./$(APP) data/config.yaml

gen-config: ## Generate data/config.yaml (SITE_URL=... ADMIN_USER=... make gen-config)
	@mkdir -p data
	go build -o $(APP) $(CMD)
	./$(APP) genconfig \
		--site-url=$${SITE_URL:-http://localhost:8080} \
		--admin-user=$${ADMIN_USER:-admin} \
		--db=$${DB_PATH:-data/u.db} \
		--addr=$${ADDR:-:8080} \
		--output=data/config.yaml

clean: ## Remove build artifacts
	rm -f $(APP)

deps: ## Download Go module dependencies
	go mod download

test: ## Run all tests
	go test -v ./...

test-coverage: ## Run tests with coverage report (opens in browser)
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

hashpw: build ## Generate bcrypt hash for a password (PASSWORD=yourpass)
	./$(APP) hashpw $${PASSWORD:-changeme}

migrate: ## Import YOURLS MySQL dump into SQLite (DUMP=dump.sql DB=data/u.db)
	@mkdir -p data
	python3 migrate.py --dump $${DUMP:-dump.sql} --db $${DB:-data/u.db}
