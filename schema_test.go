package gateway

import (
	"encoding/json"
	"testing"

	"github.com/alecaivazis/graphql-gateway/graphql"
	"github.com/stretchr/testify/assert"
)

func TestSchemaIntrospection(t *testing.T) {
	t.Skip()
	schema, _ := graphql.LoadSchema(`
		type User {
			firstName: String!
			lastName: String!
		}

		type Query {
			allUsers: [User]
		}
	`)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url1"},
	})
	if err != nil {
		t.Error(err.Error())
		return
	}

	// executing the introspection query should return a full description of the schema
	response, err := gateway.Execute(graphql.IntrospectionQuery)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// a place to hold the marshaled version
	responseMarshaled, err := json.Marshal(response)
	if err != nil {
		t.Error(err.Error())
		return
	}

	expectedMarshaled, err := json.Marshal(&graphql.IntrospectionQueryResult{
		Schema: &graphql.IntrospectionQuerySchema{
			QueryType: graphql.IntrospectionQueryRootType{
				Name: "Query",
			},
			Types: []graphql.IntrospectionQueryFullType{
				{
					Name: "Query",
					Fields: []graphql.IntrospectionQueryFullTypeField{
						{
							Name: "allUsers",
							Type: graphql.IntrospectionTypeRef{
								Kind: "LIST",
								OfType: &graphql.IntrospectionTypeRef{
									Kind: "OBJECT",
									Name: "User",
								},
							},
						}},
				},
				{
					Name: "User",
					Fields: []graphql.IntrospectionQueryFullTypeField{
						{
							Name: "firstName",
							Type: graphql.IntrospectionTypeRef{
								Kind: "SCALAR",
								Name: "String",
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure the 2 matched
	assert.Equal(t, string(expectedMarshaled), string(responseMarshaled))
}
