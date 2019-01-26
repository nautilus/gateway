# graphql-gateway

[![Build Status](https://travis-ci.com/AlecAivazis/graphql-gateway.svg?branch=master)](https://travis-ci.com/AlecAivazis/graphql-gateway) [![Coverage Status](https://coveralls.io/repos/github/AlecAivazis/graphql-gateway/badge.svg?branch=master)](https://coveralls.io/github/AlecAivazis/graphql-gateway?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/alecaivazis/graphql-gateway)](https://goreportcard.com/report/github.com/alecaivazis/graphql-gateway)

A standalone service designed to consolidate your graphql APIs into one endpoint.

For a more detailed description of this project's
motivation read [this post](). For a guide to getting started read [this post]()

## Running the Executable Directly

The simplest way to run a gateway is to download the executable
from the latest release on GitHub and then run it directly on
your machine:

```bash
$ ./graphql-gateway start --port 4000 --services http://localhost:3000,http://localhost:3001
```

This will start a server on port 4000 that wraps over the services
running at `http://localhost:3000` and `http://localhost:3001`. For more information on possible
arguments to pass the executable, run `./graphql-gateway --help`.

## Customizing the Gateway

While the executable is good for getting started quickly, it isn't sufficient for
most production usecases. Unfortunately, there is currently no story to plug in custom
authentication/authorization logic or other extensions when running a gateway with the
cli. If being able to run a custom gateway with the cli is something that interests you,
please open in an issue! For now, these common situations require building your own executable.

The core object of the gateway is the `Gateway` struct exported by the module at
`github.com/alecaivazis/graphql-gateway`. A `Gateway` is constructed by providing
a list of `graphql.RemoteSchema`s for each service to `gateway.New`. The easiest way to
get a `graphql.RemoteSchema` is to introspect the remote schema using a utility from
`github.com/alecaivazis/graphql-gateway/graphql`:

```golang
package main

import (
	gateway "github.com/alecaivazis/graphql-gateway"
	"github.com/alecaivazis/graphql-gateway/graphql"
)

func main() {
	// introspect the apis
	schemas, err := graphql.IntrospectRemoteSchemas(
		"http://localhost:3000",
		"http://localhost:3001",
    	)
	if err != nil {
		panic(err)
	}

	// create the gateway instance
	gateway, err := gateway.New(schemas)
	if err != nil {
		panic(err)
	}
}
```

### Integrating with a different HTTP Server

A `Gateway` provides 2 different handlers which are both instances of `http.HandlerFunc` so they should easily 
integrate into whichever web server you prefer:

- `gateway.GraphQLHandler` responds to both `GET` and `POST` requests as described
  [in the spec](https://graphql.org/learn/serving-over-http/).
- `gateway.PlaygroundHandler` responds to `GET` requests with a web-based IDE for easy exploration
  and interprets `POST`s as queries to process.

```golang
package main

import (
	"fmt"
	"net/http"

	// ... including the ones above
)

func main() {
	// ... including up above

	// add the playground endpoint to the router
	http.HandleFunc("/graphql", gateway.PlaygroundHandler)

	// start the server
	fmt.Println("Starting server")
	err = http.ListenAndServe(":3001", nil)
	if err != nil {
		fmt.Println(err.Error())
	}
}
```

### Modifying Service Requests

There are many situations where one might want to modify the network requests sent from 
the gateway to the other services. In order to do this, you can define a `RequestMiddleware`
that will be called for every request sent:

```golang
addHeader := gateway.RequestMiddleware(func(r *http.Request) (error) {
	r.Header.Set("AwesomeHeader", "MyValue")

	// return the modified request
	return nil
})

// ... somewhere else ...

gateway.New(..., gateway.withMiddleware(addHeader))
```

The context of this request is the same context of the incoming network request. If you 
want to pull something out of the incoming network request (its IP for example) and add 
it to the outbound requests, you would write it in 2 parts


#### Authentication and Authorization

Currently the gateway has no opinion on a method for authentication and authorization.
Descisions for wether a user can or cannot do something is pushed down to the services
handling the queries. To pass information about the current request to the backing
services, information can be attach to the request context and then used when executing
a query.

See the [auth example](./examples/auth) for more information.

