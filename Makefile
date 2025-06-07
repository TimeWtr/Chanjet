.PHONY: setup
setup:
	@sh ./scripts/setup.sh


.PHONY: tidy
tidy:
	@go mod tidy

.PHONY: ut
ut:
	@go test -race ./...

.PHONY: lint
lint:
	@golangci-lint run -c ./scripts/lint/.golangci.yml ./...

.PHONY: check
check:
	@$(MAKE) --no-print-directory setup
	@$(MAKE) --no-print-directory tidy
	@$(MAKE) --no-print-directory ut
	@$(MAKE) --no-print-directory lint