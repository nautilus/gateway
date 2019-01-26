package main

import (
	"context"
	"fmt"
	"net/http"

	gateway "github.com/alecaivazis/graphql-gateway"
	"github.com/alecaivazis/graphql-gateway/graphql"
)

func main() {
	// introspect the apis
	schemas, err := graphql.IntrospectRemoteSchemas(
		"http://localhost:8080/",
		"http://localhost:8081/",
	)
	if err != nil {
		panic(err)
	}

	// we can define a request pipeline that each request to another service goes through
	requestPipeline := &gateway.RequestPipeline{
		// we only need to do one thing with this pipeline: add the value of the Authorization
		// header to the outbound requests
		func(ctx context.Context, r *http.Request) *http.Request {
			// grab the value of the incoming Authorization header
			incomingRequest := ctx.Value("incoming-request").(*http.Request)

			// set the outbound USER_ID header to match the inbound Authorization header
			r.Header().Set("USER_ID", incomingRequest.Header().Get("Authorization"))

			// return the modified request
			return r
		},
	}

	// create the gateway instance
	gw, err := gateway.New(schemas, gateway.WithRequestPipeline(requestPipeline))
	if err != nil {
		panic(err)
	}

	// add the playground endpoint to the router
	http.HandleFunc("/graphql", gw.PlaygroundHandler)

	// start the server
	fmt.Println("Starting server")
	err = http.ListenAndServe(":3001", nil)
	if err != nil {
		fmt.Println(err.Error())
	}
}
