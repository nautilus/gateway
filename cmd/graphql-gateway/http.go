package main

import (
	"fmt"
	"net/http"

	gateway "github.com/alecaivazis/graphql-gateway"
	"github.com/alecaivazis/graphql-gateway/graphql"
)

func ListenAndServe(services []string) {
	// build up the list of remote schemas
	schemas := []*graphql.RemoteSchema{}

	for _, service := range services {
		// introspect the locations
		schema, err := graphql.IntrospectRemoteSchema(service)
		if err != nil {
			panic(err)
		}

		// add the schema to the list
		schemas = append(schemas, schema)
	}

	// create the gateway instance
	gw, err := gateway.New(schemas)
	if err != nil {
		panic(err)
	}

	// add the graphql endpoints to the router
	http.HandleFunc("/graphql", setCORSHeaders(gw.PlaygroundHandler))

	// start the server
	fmt.Println("Starting server")
	err = http.ListenAndServe(fmt.Sprintf(":%s", Port), nil)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func setCORSHeaders(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// set the necessary CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET,HEAD,OPTIONS,POST,PUT")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		// if we are handling a pre-flight request
		if req.Method == http.MethodOptions {
			return
		}

		// invoke the handler
		fn(w, req)
	}
}
