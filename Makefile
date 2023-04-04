
.PHONY: test
test:
	go test -race -coverprofile=cover.out ./...
	cd ./cmd/gateway && go test -v -race ./...

.PHONY: install-deps
install-deps:
	# Install the necessary dependencies to run in CI.
	go install github.com/mitchellh/gox@latest

.PHONY: build
build: install-deps
	set -ex; \
		cd cmd/gateway; \
		go mod edit -replace github.com/nautilus/gateway=../..; \
		go mod download github.com/nautilus/gateway; \
		gox -os="linux darwin windows" -arch=amd64 -output="../../bin/gateway_{{.OS}}_{{.Arch}}" -verbose .
