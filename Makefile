.PHONY: build test set-version
build: set-version
	cd web && npm ci && npm run build
	mkdir -p dist
	go build -o dist/example .
	python3 scripts/package.py

set-version:
	@if [ -n "$(VERSION)" ]; then python3 scripts/set_version.py "$(VERSION)"; fi

test:
	go test ./...
	cd web && npm test
