module github.com/nautilus/gateway/cmd/gateway

require (
	github.com/nautilus/gateway v0.1.19
	github.com/nautilus/graphql v0.0.19
	github.com/spf13/cobra v0.0.5
	golang.org/x/sys v0.0.0-20220615213510-4f61da869c0c // indirect
)

go 1.13

replace github.com/nautilus/gateway => ../..
