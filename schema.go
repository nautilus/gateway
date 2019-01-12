package gateway

import (
	"fmt"

	"github.com/alecaivazis/graphql-gateway/graphql"
)

// internalSchema is a graphql schema that exists at the gateway level and is merged with the
// other schemas that the gateway wraps.
var internalSchema *graphql.RemoteSchema

// internalSchemaLocation is the location that functions should take to identify a remote schema
// that points to the gateway's internal schema.
const internalSchemaLocation = "🎉"

func init() {
	schema, err := graphql.LoadSchema(`
		type Query {
			_apiVersion: String
		}
	`)
	if schema == nil {
		panic(fmt.Sprintf("Syntax error in schema string: %s", err.Error()))
	}

	internalSchema = &graphql.RemoteSchema{
		URL:    internalSchemaLocation,
		Schema: schema,
	}
}
