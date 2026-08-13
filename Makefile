API_DIR  := router/api
WEB_DIR  := router/web
PNPM     := pnpm

.PHONY: fmt lint test build

# fmt: reformat Go (gofumpt + goimports via golangci-lint) and web (oxfmt).
fmt:
	cd $(API_DIR) && golangci-lint fmt
	$(PNPM) --filter @router/web run format

# lint: static-analysis for Go (golangci-lint) and web (oxlint).
lint:
	cd $(API_DIR) && golangci-lint run
	$(PNPM) --filter @router/web run lint

# test: Go unit tests and TypeScript type-check (tsc -b used as a build-less typecheck).
test:
	cd $(API_DIR) && go test ./...
	$(PNPM) --filter @router/web run build

# build: compile Go binary and bundle the web UI.
build:
	cd $(API_DIR) && go build ./...
	$(PNPM) --filter @router/web run build
