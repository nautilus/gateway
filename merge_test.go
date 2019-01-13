package gateway

import (
	"testing"

	"github.com/alecaivazis/graphql-gateway/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

func TestMergeSchema_fields(t *testing.T) {
	// create the first schema
	schema1, err := graphql.LoadSchema(`
			type User {
				firstName: String!
			}
	`)

	// make sure nothing went wrong
	assert.Nil(t, err)

	// and the second schema we are going to make
	schema2, err := graphql.LoadSchema(`
			type User {
				lastName: String!
			}
	`)
	// make sure nothing went wrong
	assert.Nil(t, err)

	// merge the schemas together
	schema, err := New([]*graphql.RemoteSchema{
		&graphql.RemoteSchema{Schema: schema1, URL: "url1"},
		&graphql.RemoteSchema{Schema: schema2, URL: "url2"},
	})
	// make sure nothing went wrong
	assert.Nil(t, err)

	// look up the definition for the User type
	definition, exists := schema.schema.Types["User"]
	// make sure the definition exists
	assert.True(t, exists)

	// it should have 2 fields: firstName and lastName
	var firstNameDefinition *ast.FieldDefinition
	var lastNameDefinition *ast.FieldDefinition

	// look for the definitions
	for _, field := range definition.Fields {
		if field.Name == "firstName" {
			firstNameDefinition = field
		} else if field.Name == "lastName" {
			lastNameDefinition = field
		}
	}

	// make sure the firstName definition exists
	if firstNameDefinition == nil {
		t.Error("could not find definition for first name")
		return
	}
	assert.Equal(t, "String!", firstNameDefinition.Type.String())

	// make sure the lastName definition exists
	if lastNameDefinition == nil {
		t.Error("could not find definition for last name")
		return
	}
	assert.Equal(t, "String!", lastNameDefinition.Type.String())
}

func TestMergeSchema_assignQueryType(t *testing.T) {
	// create the first schema
	schema1, err := graphql.LoadSchema(`
			type Query {
				firstName: String!
			}
	`)

	// make sure nothing went wrong
	assert.Nil(t, err)

	// and the second schema we are going to make
	schema2, err := graphql.LoadSchema(`
			type Query {
				lastName: String!
			}
	`)
	// make sure nothing went wrong
	assert.Nil(t, err)

	// merge the schemas together
	schema, err := New([]*graphql.RemoteSchema{
		&graphql.RemoteSchema{Schema: schema1, URL: "url1"},
		&graphql.RemoteSchema{Schema: schema2, URL: "url2"},
	})
	// make sure nothing went wrong
	assert.Nil(t, err)

	// look up the definition for the User type
	definition := schema.schema.Query
	if definition == nil {
		t.Error("Could not find a query type")
	}
}

func TestMergeSchema_assignMutationType(t *testing.T) {
	// create the first schema
	schema1, err := graphql.LoadSchema(`
			type Mutation {
				firstName: String!
			}
	`)

	// make sure nothing went wrong
	assert.Nil(t, err)

	// and the second schema we are going to make
	schema2, err := graphql.LoadSchema(`
			type Mutation {
				lastName: String!
			}
	`)
	// make sure nothing went wrong
	assert.Nil(t, err)

	// merge the schemas together
	schema, err := New([]*graphql.RemoteSchema{
		&graphql.RemoteSchema{Schema: schema1, URL: "url1"},
		&graphql.RemoteSchema{Schema: schema2, URL: "url2"},
	})
	// make sure nothing went wrong
	assert.Nil(t, err)

	// look up the definition for the User type
	definition := schema.schema.Mutation
	if definition == nil {
		t.Error("Could not find a Mutation type")
	}
}

func TestMergeSchema_conflictingFieldTypes(t *testing.T) {
	// create the first schema
	schema1, err := graphql.LoadSchema(`
			type User {
				firstName: String!
			}
	`)

	// make sure nothing went wrong
	assert.Nil(t, err)

	// and the second schema we are going to make
	schema2, err := graphql.LoadSchema(`
			type User {
				firstName: Int
			}
	`)
	// make sure nothing went wrong
	assert.Nil(t, err)

	// merge the schemas together
	_, err = New([]*graphql.RemoteSchema{
		&graphql.RemoteSchema{Schema: schema1, URL: "url1"},
		&graphql.RemoteSchema{Schema: schema2, URL: "url2"},
	})
	// make sure nothing went wrong
	if err == nil {
		t.Error("didn't encounter error while merging schemas")
		return
	}
}
