package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/nautilus/gateway"
	"github.com/nautilus/graphql"
)

func ListenAndServe(services []string) {
	// introspect the schemas
	schemas, err := graphql.IntrospectRemoteSchemas(services...)
	if err != nil {
		fmt.Println("Encountered error introspecting schemas:", err.Error())
		os.Exit(1)
	}

	// create the gateway instance
	gw, err := gateway.New(schemas)
	if err != nil {
		fmt.Println("Encountered error starting gateway:", err.Error())
		os.Exit(1)
	}

	// add the graphql endpoints to the router
	http.HandleFunc("/graphql", setCORSHeaders(gw.PlaygroundHandler))

	// start the server
	fmt.Printf("🚀 Gateway is ready at http://localhost:%s/graphql\n", Port)
	err = http.ListenAndServe(fmt.Sprintf(":%s", Port), nil)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
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
