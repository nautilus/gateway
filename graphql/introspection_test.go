package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntrospectQuery_savesQueryType(t *testing.T) {
	// introspect the api with a known response
	schema, err := IntrospectAPI(&MockQueryer{
		IntrospectionQueryResult{
			Schema: &IntrospectionQuerySchema{
				QueryType: &IntrospectionQueryRootType{
					Name: "Query",
				},
			},
		},
	})
	// if something went wrong
	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure we got a schema back
	if schema == nil {
		t.Error("Received nil schema")
		return
	}

	// make sure the query type has the right name
	assert.Equal(t, "Query", schema.Query.Name)
}
