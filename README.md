# graphql-gateway

[![Build Status](https://travis-ci.com/AlecAivazis/graphql-gateway.svg?branch=master)](https://travis-ci.com/AlecAivazis/graphql-gateway) [![Coverage Status](https://coveralls.io/repos/github/AlecAivazis/graphql-gateway/badge.svg?branch=master)](https://coveralls.io/github/AlecAivazis/graphql-gateway?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/alecaivazis/graphql-gateway)](https://goreportcard.com/report/github.com/alecaivazis/graphql-gateway)

A standalone service designed to consolidate your graphql APIs into one endpoint.

For a more detailed description of this project's
motivation read [this post](). For a guide to getting started read [this post]().

# Running the Executable Directly

The simplest way to run a gateway is to download the executable
from the latest release on GitHub and then run it directly on
your machine:

```bash
$ ./graphql-gateway start --port 4000 --services http://localhost:3000,http://localhost:3001
```

This will start a server on port 4000 that wraps over the services
running at `http://localhost:3000` and `http://localhost:3001`. For more information on possible
arguments to pass the executable, run `./graphql-gateway --help`.

# Customizing the Gateway

While the executable is good for getting started quickly, it isn't sufficient for
most production usecases. Unfortunately, there is currently no story to plug in custom
authentication/authorization logic or other extensions when running a gateway with the 
cli. If being able to run a custom gateway with the cli is something that interests you, 
please share in an issue!. For now, these common situations require building your own executable.
Doing so requires a go environment set up on your machine but if you've never written go before, 
don't worry! It's a pretty simple language and there are a few [examples](./examples) that you can follow.

The core object of the gateway is the `gateway.Gateway` struct exported by the module at 
`github.com/alecaivazis/graphql-gateway`.

