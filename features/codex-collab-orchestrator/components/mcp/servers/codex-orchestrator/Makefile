GO := ./scripts/go.sh
GOFMT := ./scripts/gofmt.sh
REPO_ROOT ?= ../../../../..

.PHONY: test tidy fmt run-serve run-init

test:
	$(GO) test ./...

tidy:
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...
	$(GOFMT) -w cmd internal

run-serve:
	$(GO) run ./cmd/codex-orchestrator --mode serve --repo $(REPO_ROOT)

run-init:
	$(GO) run ./cmd/codex-orchestrator --mode once --method workspace.init --params '{}' --repo $(REPO_ROOT)
