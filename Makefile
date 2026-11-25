GO ?= go
PYTHON ?= python3
STATICCHECK_VERSION := v0.8.2
GOVULNCHECK_VERSION := v1.10.0
STATICCHECK := $(CURDIR)/.tools/bin/staticcheck-$(STATICCHECK_VERSION)
GOVULNCHECK := $(CURDIR)/.tools/bin/govulncheck-$(GOVULNCHECK_VERSION)

.PHONY: help check lint coverage test test-tooling integration build run secrets vuln tidy-check clean

help:
	@printf '%s\n' 'check: lint, coverage/race tests, tooling regressions, vulnerability scans' 'integration: all tests against a fresh task-owned Mongo replica set' 'build/run: build the Linux binary or run the local API' 'secrets: print new JWT keys locally; never applies them' 'image-scan: build/probe image and scan OS HIGH/CRITICAL findings' 'config-check/staging-mounts/ingress-check: read-only explicit staging gates' 'release-check: all automated gates; never deploys or rotates keys' 'clean: remove generated build reports and binaries'

check: lint coverage test-tooling vuln

lint: $(STATICCHECK) tidy-check
	$(GO) vet ./...
	@mkdir -p build
	PATH="$$(GOTOOLCHAIN=go1.26.11 $(GO) env GOROOT)/bin:$$PATH" GOTOOLCHAIN=local STATICCHECK_CACHE="$(CURDIR)/build/staticcheck-cache" $(PYTHON) scripts/quality_gate.py staticcheck --binary "$(STATICCHECK)" --report build/staticcheck.json

tidy-check:
	$(GO) mod tidy -diff

coverage:
	@mkdir -p build
	$(GO) test -race -coverpkg=./... -covermode=atomic -coverprofile=build/coverage.out ./... -count=1
	$(PYTHON) scripts/quality_gate.py coverage --profile build/coverage.out

test:
	$(GO) test -race ./... -count=1

test-tooling:
	$(PYTHON) -m unittest discover -s tests -p 'test_*.py'

integration:
	bash scripts/test-workflow-mongo-integration.sh -race

build:
	@mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -o build/autotable .

run:
	$(GO) run .

secrets:
	$(GO) run ./cmd/autotable-secrets

vuln: $(GOVULNCHECK)
	PATH="$$(GOTOOLCHAIN=go1.26.11 $(GO) env GOROOT)/bin:$$PATH" GOTOOLCHAIN=local "$(GOVULNCHECK)" ./...
	PATH="$$(GOTOOLCHAIN=go1.26.11 $(GO) env GOROOT)/bin:$$PATH" GOTOOLCHAIN=local "$(GOVULNCHECK)" -scan=module

$(STATICCHECK):
	@mkdir -p .tools/bin
	GOTOOLCHAIN=go1.26.11 GOBIN="$(CURDIR)/.tools/bin" $(GO) install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	mv .tools/bin/staticcheck "$@"

$(GOVULNCHECK):
	@mkdir -p .tools/bin
	GOTOOLCHAIN=go1.26.11 GOBIN="$(CURDIR)/.tools/bin" $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	mv .tools/bin/govulncheck "$@"

clean:
	rm -rf -- build

.PHONY: image-check image-scan staging-mounts
image-check:
	bash scripts/check-container.sh image

image-scan: image-check
	bash scripts/check-container.sh scan

staging-mounts:
	bash scripts/check-container.sh mounts

.PHONY: config-check
config-check:
	$(GO) run ./cmd/autotable-secrets --check

.PHONY: ingress-check release-check
ingress-check:
	$(PYTHON) scripts/check-ingress.py --api-url "$${STAGING_API_URL:?set STAGING_API_URL to an HTTPS staging origin}" --frontend-url "$${STAGING_FRONTEND_URL:?set STAGING_FRONTEND_URL to its HTTPS frontend}"

# Verification only. No target deploys, rotates credentials or changes mount ownership.
release-check: check integration config-check image-scan
	$(MAKE) staging-mounts
	$(MAKE) ingress-check
