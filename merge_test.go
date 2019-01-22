package gateway

import (
	"testing"

	"github.com/alecaivazis/graphql-gateway/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

func TestMergeSchema_objectTypeFields(t *testing.T) {
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
		{Schema: schema1, URL: "url1"},
		{Schema: schema2, URL: "url2"},
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
		{Schema: schema1, URL: "url1"},
		{Schema: schema2, URL: "url2"},
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
		{Schema: schema1, URL: "url1"},
		{Schema: schema2, URL: "url2"},
	})
	// make sure nothing went wrong
	assert.Nil(t, err)

	// look up the definition for the User type
	definition := schema.schema.Mutation
	if definition == nil {
		t.Error("Could not find a Mutation type")
	}
}

func TestMergeSchema_objectTypes(t *testing.T) {
	// create the first schema
	schema1, err := graphql.LoadSchema(`
			type User {
				firstName: String!
			}
	`)

	// make sure nothing went wrong
	assert.Nil(t, err)

	// the table we are testing
	testRunMergeTable(t, schema1, []testMergeTableRow{
		{
			"Conflicting Field Type",
			false,
			`
				type User {
					firstName: Int
				}
			`,
		},
	})
}

func TestMergeSchema_enums(t *testing.T) {
	// the directive that we are always comparing to
	originalSchema, err := graphql.LoadSchema(`
		enum Foo {
			Bar
			Baz
		}
	`)
	// make sure nothing went wrong
	if !assert.Nil(t, err, "original schema didn't parse") {
		return
	}

	// the table we are testing
	testRunMergeTable(t, originalSchema, []testMergeTableRow{
		{
			"Conflicting Names",
			false, `
				enum Foo {
					Bar
				}
			`,
		},
	})
}

func TestMergeSchema_directives(t *testing.T) {
	// the directive that we are always comparing to
	originalSchema, err := graphql.LoadSchema(`
		"description"
		directive @foo(url: String = "url") on FIELD_DEFINITION
	`)
	// make sure nothing went wrong
	if !assert.Nil(t, err, "original schema didn't parse") {
		return
	}

	// run the table of tests
	testRunMergeTable(t, originalSchema, []testMergeTableRow{
		{
			"Matching",
			true,
			`
				"description"
				directive @foo(url: String = "url") on FIELD_DEFINITION
			`,
		},
		{
			"Different Argument Type",
			false,
			`
				"description"
				directive @foo(url: String! = "url") on FIELD_DEFINITION
			`,
		},
		{
			"Different Arguments",
			false,
			`
				"description"
				directive @foo(url: String = "url", number: Int) on FIELD_DEFINITION
			`,
		},
		{
			"Different Location",
			false,
			`
				"description"
				directive @foo(url: String = "url") on FRAGMENT_SPREAD
			`,
		},
		{
			"Different Number of Locations",
			false,
			`
				"description"
				directive @foo(url: String = "url") on FRAGMENT_SPREAD | FIELD_DEFINITION
			`,
		},
		{
			"Different Description",
			false,
			`
				"other description"
				directive @foo(url: String = "url") on FIELD_DEFINITION
			`,
		},
		{
			"Different Default Value",
			false,
			`
				"description"
				directive @foo(url: String = "not-url") on FIELD_DEFINITION
			`,
		},
	})
}

func TestMergeSchema_interfaces(t *testing.T) {
	// the directive that we are always comparing to
	originalSchema, err := graphql.LoadSchema(`
		interface Foo {
			name: String!
		}
	`)
	// make sure nothing went wrong
	if !assert.Nil(t, err, "original schema didn't parse") {
		return
	}

	// the table we are testing
	testRunMergeTable(t, originalSchema, []testMergeTableRow{
		{
			"Matching",
			true,
			`
				interface Foo {
					name: String!
				}
			`,
		},
		{
			"Different Field Directives",
			false,
			`
				directive @foo on FIELD_DEFINITION

				interface Foo {
					name: String! @foo
				}
			`,
		},
		{
			"Different Field Types",
			false,
			`
				interface Foo {
					name: String
				}
			`,
		},
		{
			"Different Fields",
			false,
			`
				interface Foo {
					name: String!
					lastName: String!
				}
			`,
		},
		{
			"Different Arguments",
			false,
			`
				interface Foo {
					name(foo: String): String!
				}
			`,
		},
	})
}

type testMergeTableRow struct {
	Message string
	Pass    bool
	Schema  string
}

func testRunMergeTable(t *testing.T, original *ast.Schema, table []testMergeTableRow) {
	for _, row := range table {
		t.Run(row.Message, func(t *testing.T) {
			// create a schema with the provided content
			schema2, err := graphql.LoadSchema(row.Schema)
			// make sure nothing went wrong
			if !assert.Nil(t, err, "comparison schema didn't parse") {
				return
			}
			// create remote schemas with each
			_, err = New([]*graphql.RemoteSchema{
				{Schema: original, URL: "url1"},
				{Schema: schema2, URL: "url2"},
			})
			// if we were supposed to pass and didn't
			if row.Pass && err != nil {
				t.Errorf("Encountered error: %v", err.Error())
			}
			// if we were not supposed to pass and didn't encounter an error
			if !row.Pass && err == nil {
				t.Error("Did not encounter an error when one was expected")
			}
		})
	}
}
