
.PHONY: test
test:
	go test -race -coverprofile=cover.out ./...
	# cmd/gateway depends on a version tag only present in the upstream (nautilus/gateway) repo,
	# not in this fork. Use a local replace so Go resolves the module from the working tree.
	cd ./cmd/gateway && go mod edit -replace github.com/amboss-mededu/gateway=../.. && go test -v -race ./...

.PHONY: build-setup
build-setup:
	# When building cmd/gateway in CI, always use the current version of the gateway.
	set -ex; \
		cd cmd/gateway; \
		go mod edit -replace github.com/amboss-mededu/gateway=../..; \
		go mod download github.com/amboss-mededu/gateway
	rm -rf bin && mkdir bin

.PHONY: build
build: build-linux build-darwin build-windows

.PHONY: build-linux
build-linux: build-setup
	cd cmd/gateway && CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o ../../bin/gateway_linux_amd64 .

.PHONY: build-darwin
build-darwin: build-setup
	cd cmd/gateway && CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -o ../../bin/gateway_darwin_amd64 .

.PHONY: build-windows
build-windows: build-setup
	cd cmd/gateway && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ../../bin/gateway_windows_amd64.exe .
