package main

import (
	"fmt"
	"net/http"

	gateway "github.com/alecaivazis/graphql-gateway"
	"github.com/alecaivazis/graphql-gateway/graphql"
)

func main() {
	// introspect the api
	serviceASchema, err := graphql.IntrospectRemoteSchema("http://localhost:4000")
	if err != nil {
		panic(err)
	}

	// introspect the api
	serviceBSchema, err := graphql.IntrospectRemoteSchema("http://localhost:4001")
	if err != nil {
		panic(err)
	}

	// the list of remote schemas that the gateway wraps
	remoteSchemas := []*graphql.RemoteSchema{serviceASchema, serviceBSchema}

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
	err = http.ListenAndServe(":3001", nil)
	if err != nil {
		fmt.Println(err.Error())
	}
}
