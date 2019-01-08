package main

import (
	"github.com/alecaivazis/graphql-gateway/graphql"
)

func main() {
	// the queryer to hit the api
	queryer := graphql.NewNetworkQueryer("http://localhost:3000")

	// introspect the api
	_, err := graphql.IntrospectAPI(queryer)
	if err != nil {
		panic(err)
	}
}
