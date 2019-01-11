package main

import (
	"fmt"
	"net/http"

	gateway "github.com/alecaivazis/graphql-gateway"
	"github.com/alecaivazis/graphql-gateway/graphql"
)

func main() {

	// introspect the api
	serviceASchema, err := graphql.IntrospectAPI(graphql.NewNetworkQueryer("http://localhost:4000"))
	if err != nil {
		panic(err)
	}

	// introspect the api
	serviceBSchema, err := graphql.IntrospectAPI(graphql.NewNetworkQueryer("http://localhost:4001"))
	if err != nil {
		panic(err)
	}

	// the list of remote schemas that the gateway wraps
	remoteSchemas := []graphql.RemoteSchema{
		{
			Schema: serviceASchema,
			URL:    "http://localhost:4000",
		},
		{
			Schema: serviceBSchema,
			URL:    "http://localhost:4001",
		},
	}

	// create the gateway instance
	gatewaySchema, err := gateway.New(remoteSchemas)
	if err != nil {
		panic(err)
	}

	// add the graphql endpoints
	http.HandleFunc("/graphql", gatewaySchema.GraphQLHandler)

	// log the user
	fmt.Println("Starting server")

	// start the server
	http.ListenAndServe(":3001", nil)
}
