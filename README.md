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
running at localhost:3000 and localhost:3001. For more information on possible
arguments to pass the executable, run `./graphql-gateway --help`.

# Customizing the Gateway

While the executable is good for getting started quickly, it isn't sufficient for
most production usecases. Unfotunately, there is not currently a good story to plug in custom
authentication/authorization logic and other extensions when running from the cli.
Those scenarios require compiling your own executable.
