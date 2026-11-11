# The product's one entry point for checks. A stub that loom's `ci` job runs until the product's first milestone
# builds the real one: every Go module's format, vet and tests, and any tests/test_*.sh.
.PHONY: ci
ci:
	@set -e; for m in $$(find . -name go.mod -not -path './.git/*' -exec dirname {} \;); do \
		echo "== $$m"; (cd "$$m" && test -z "$$(gofmt -l .)" && go vet ./... && go test -count=1 ./...); done
	@set -e; for t in tests/test_*.sh; do [ -e "$$t" ] || continue; echo "== $$t"; sh "$$t"; done
