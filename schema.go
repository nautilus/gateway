package gateway

import (
	"fmt"

	"github.com/alecaivazis/graphql-gateway/graphql"
	"github.com/vektah/gqlparser/ast"
)

// internalSchema is a graphql schema that exists at the gateway level and is merged with the
// other schemas that the gateway wraps.
var internalSchema *graphql.RemoteSchema

// internalSchemaLocation is the location that functions should take to identify a remote schema
// that points to the gateway's internal schema.
const internalSchemaLocation = "🎉"

// SchemaQueryer is a queryer that knows how to resolve a query according to a particular schema
type SchemaQueryer struct {
	Schema *ast.Schema
}

// Query takes a query definition and writes the result to the receiver
func (q *SchemaQueryer) Query(input *graphql.QueryInput, receiver interface{}) error {
	fmt.Println("Executing local query")
	return nil
}

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
