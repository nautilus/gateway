package main

import (
	"fmt"
	"net/http"

	gateway "github.com/alecaivazis/graphql-gateway"
	"github.com/alecaivazis/graphql-gateway/graphql"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the gateway",
	Run:   startServer,
}

var Port string

func init() {
	startCmd.Flags().StringVarP(&Port, "port", "p", "", "The port to listen on.")

}

func startServer(cmd *cobra.Command, args []string) {
	// introspect the apis
	serviceASchema, err := graphql.IntrospectRemoteSchema("http://localhost:4000")
	if err != nil {
		panic(err)
	}
	serviceBSchema, err := graphql.IntrospectRemoteSchema("http://localhost:4001")
	if err != nil {
		panic(err)
	}

	// create the gateway instance
	gateway, err := gateway.New([]*graphql.RemoteSchema{serviceASchema, serviceBSchema})
	if err != nil {
		panic(err)
	}

	// add the graphql endpoints to the router
	http.HandleFunc("/graphql", setCORSHeaders(gateway.PlaygroundHandler))

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
