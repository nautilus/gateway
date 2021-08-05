module github.com/nautilus/gateway/cmd

require (
	github.com/nautilus/gateway v0.0.0-00010101000000-000000000000
	github.com/nautilus/graphql v0.0.16
	github.com/spf13/cobra v0.0.5
)

replace github.com/nautilus/gateway => ../

go 1.13
