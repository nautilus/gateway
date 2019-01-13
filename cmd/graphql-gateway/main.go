package main

import (
	"fmt"
	"net/http"

	gateway "github.com/alecaivazis/graphql-gateway"
	"github.com/alecaivazis/graphql-gateway/graphql"
)

func enableCors(fn http.HandlerFunc) http.HandlerFunc {
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

	// create the gateway instance
	gateway, err := gateway.New([]*graphql.RemoteSchema{serviceASchema, serviceBSchema})
	if err != nil {
		panic(err)
	}

	// add the graphql endpoints
	http.HandleFunc("/graphql", enableCors(gateway.GraphiQLHandler))

	// log the user
	fmt.Println("Starting server")

	// start the server
	err = http.ListenAndServe(":3001", nil)
	if err != nil {
		fmt.Println(err.Error())
	}
}
