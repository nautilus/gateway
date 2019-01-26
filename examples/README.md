# Examples

This directory contains examples of different things one can do with a
custom gateway to fit their needs.

Each example contains a gateway and one or more services that showcase the
example. To run each example, start each service in its own terminal. For example,
the run the `auth` example, you'll have to start 3 separate processes, `gateway.go`,
`users.go`, and `todo.go`. All of them can be run with `go run <filename>`.

Please keep in mind that the non-gateway services do not have to be
written in go. All of these examples work regardless of the language you chose
to write your APIs.

List of Examples:

- [Hello World](./hello): A simple proof of concept
- Something a bit more complex (Coming Soon):  A more realistic example of an e-commerce platform
- [Authentication and Authorization](./auth): An overview of how to support auth behind a gateway
- Watch Remote Schemas (Coming Soon): A way to support CD workflows
