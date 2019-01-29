package main

import (
	"fmt"
	"net/http"

	"github.com/nautilus/gateway"
	"github.com/nautilus/graphql"
)

func ListenAndServe(services []string) {
	// introspect the schemas
	schemas, err := graphql.IntrospectRemoteSchemas(services...)
	if err != nil {
		panic(err)
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
