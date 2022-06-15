# nautilus/gateway

![CI Checks](https://github.com/nautilus/gateway/workflows/CI%20Checks/badge.svg?branch=master) [![Coverage Status](https://coveralls.io/repos/github/nautilus/gateway/badge.svg?branch=master)](https://coveralls.io/github/nautilus/gateway?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/nautilus/gateway)](https://goreportcard.com/report/github.com/nautilus/gateway)

A standalone service designed to consolidate your graphql APIs into one endpoint.

For a guide to getting started read [this post](https://medium.com/@aaivazis/a-guide-to-schema-federation-part-1-995b639ac035). For full documentation visit the [gateway homepage](https://gateway.nautilus.dev).

> Note: We are looking for some help maintaing this project. If you are interested, please reach out. For a more actively maintained alternative,
> check out [Bramble](https://github.com/movio/bramble).

## Running the Executable

The simplest way to run a gateway is to download the executable
from the latest release on GitHub and then run it directly on
your machine:

```bash
$ ./gateway start --port 4000 --services http://localhost:3000,http://localhost:3001
```

Alternatively, install it with the `go` command to your Go bin and run it:
```bash
$ go install github.com/nautilus/gateway/cmd/gateway@latest
$ gateway start --port 4000 --services http://localhost:3000,http://localhost:3001
```

This will start a server on port 4000 that wraps over the services
running at `http://localhost:3000` and `http://localhost:3001`. For more information on possible
arguments to pass the executable, run `./gateway --help`.

## Versioning

This project is built as a go module and follows the practices outlined in the [spec](https://github.com/golang/go/wiki/Modules). Please consider all APIs experimental and subject
to change until v1 has been released at which point semantic versioning will be strictly followed. Before
then, minor version bumps denote an API breaking change.

Currently supports Go Modules using Go 1.13 and above.
