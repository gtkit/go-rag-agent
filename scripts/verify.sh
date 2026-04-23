#!/usr/bin/env bash
set -euo pipefail

run_golangci_lint() {
	if command -v golangci-lint >/dev/null 2>&1; then
		golangci-lint run ./...
		return
	fi
	if [ -x /Users/xiaozhaofu/go/bin/golangci-lint ]; then
		/Users/xiaozhaofu/go/bin/golangci-lint run ./...
		return
	fi
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...
}

if [ -d openspec/changes/refresh-sdk-positioning-assets ]; then
	openspec validate "refresh-sdk-positioning-assets" --type change --strict --json --no-interactive
fi
openspec validate "sdk-positioning-assets" --type spec --strict --json --no-interactive
go vet ./...
run_golangci_lint
go test -race -count=1 -timeout=5m ./...
go run ./cmd/generate-sdk-positioning-baseline

if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	git diff --exit-code -- docs/baselines/sdk-positioning
fi
