package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/alecaivazis/graphql-gateway/gateway"
	"github.com/alecaivazis/graphql-gateway/graphql"
)

func main() {
	// the queryer to hit the api
	queryer := graphql.NewNetworkQueryer("http://localhost:3000")

	// introspect the api
	schema, err := graphql.IntrospectAPI(queryer)
	if err != nil {
		panic(err)
	}

	gatewaySchema, err := gateway.NewSchema([]graphql.RemoteSchema{
		graphql.RemoteSchema{
			Schema: schema,
			URL:    "http://localhost:3000",
		},
	})
	if err != nil {
		panic(err)
	}

	// for _, field := range gatewaySchema.Schema.Types["Query"].Fields {
	// 	if field.Name == "allUsers" {
	// 		fmt.Println(field)
	// 	}
	// }

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

	http.ListenAndServe(":3001", nil)
}
