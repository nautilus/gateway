package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

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
	http.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") { // rudimentary check to see if this is accessed from a browser UI
			// if calling from a UI, redirect to the UI handler
			http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
			return
		}
		gw.GraphQLHandler(w, r)
	})

	playgroundHandler := gw.StaticPlaygroundHandler(gateway.PlaygroundConfig{
		Endpoint: "/graphql",
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			// ensure our catch-all handler pattern "/" only runs on "/"
			http.NotFound(w, r)
			return
		}

		// set the necessary CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET,HEAD,OPTIONS,POST,PUT")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		// if we are handling a pre-flight request
		if r.Method == http.MethodOptions {
			return
		}

		playgroundHandler.ServeHTTP(w, r)
	})

	// start the server
	fmt.Printf("🚀 Gateway is ready at http://localhost:%s/graphql\n", Port)
	err = http.ListenAndServe(fmt.Sprintf(":%s", Port), nil)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
