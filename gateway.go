package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/alecaivazis/graphql-gateway/gateway"
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

	gatewaySchema, err := gateway.NewSchema([]graphql.RemoteSchema{
		graphql.RemoteSchema{
			Schema: serviceASchema,
			URL:    "http://localhost:4000",
		},
		graphql.RemoteSchema{
			Schema: serviceBSchema,
			URL:    "http://localhost:4001",
		},
	})
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		query, ok := r.URL.Query()["query"]
		if !ok {
			fmt.Fprintf(w, "Please send a query to this endpoint.")
			return
		}

		result, err := gatewaySchema.Execute(query[0])
		if err != nil {
			fmt.Fprintf(w, "Encountered error during execution: %s", err.Error())
			return
		}

		payload, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintf(w, "Encountered error marshaling response: %s", err.Error())
			return
		}

		fmt.Fprintf(w, string(payload))
	})

	fmt.Println("Starting server")

	http.ListenAndServe(":3001", nil)
}
