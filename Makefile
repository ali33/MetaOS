.PHONY: test-go vet build it dev test-web e2e
test-go:  ; scripts/go.sh go test -race ./cmd/... ./internal/... ./web/
vet:      ; scripts/go.sh go vet ./cmd/... ./internal/... ./web/
build:    ; scripts/build.sh
it:       ; scripts/it.sh
dev:      ; scripts/dev.sh
test-web: ; cd web && npm test
e2e:      ; scripts/it.sh --e2e
