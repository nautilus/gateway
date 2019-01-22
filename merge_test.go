package gateway

import (
	"testing"

	"github.com/alecaivazis/graphql-gateway/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

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
	originalSchema, err := graphql.LoadSchema(`
		type User {
			firstName: String!
		}
	`)
	assert.Nil(t, err)

	t.Run("Merge fields", func(t *testing.T) {
		// merge the schema with one that should work
		schema, err := testMergeSchemas(originalSchema, `
			type User {
				lastName: String!
			}
		`)
		if err != nil {
			t.Error(err.Error())
		}

		// look up the definition for the User type
		definition, exists := schema.Types["User"]
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

	})

	// the table we are testing
	testMergeRunNegativeTable(t, originalSchema, []testMergeTableRow{
		{
			"Conflicting Field Type",
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
	testMergeRunNegativeTable(t, originalSchema, []testMergeTableRow{
		{
			"Conflicting Names", `
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

	t.Run("Matching", func(t *testing.T) {
		// merge the schema with one that should work
		_, err := testMergeSchemas(originalSchema, `
			"description"
			directive @foo(url: String = "url") on FIELD_DEFINITION
		`)
		if err != nil {
			t.Error(err.Error())
		}
	})

	// run the table of tests
	testMergeRunNegativeTable(t, originalSchema, []testMergeTableRow{
		{
			"Different Argument Type",
			`
				"description"
				directive @foo(url: String! = "url") on FIELD_DEFINITION
			`,
		},
		{
			"Different Arguments",
			`
				"description"
				directive @foo(url: String = "url", number: Int) on FIELD_DEFINITION
			`,
		},
		{
			"Different Location",
			`
				"description"
				directive @foo(url: String = "url") on FRAGMENT_SPREAD
			`,
		},
		{
			"Different Number of Locations",
			`
				"description"
				directive @foo(url: String = "url") on FRAGMENT_SPREAD | FIELD_DEFINITION
			`,
		},
		{
			"Different Description",
			`
				"other description"
				directive @foo(url: String = "url") on FIELD_DEFINITION
			`,
		},
		{
			"Different Default Value",
			`
				"description"
				directive @foo(url: String = "not-url") on FIELD_DEFINITION
			`,
		},
	})
}

func TestMergeSchema_union(t *testing.T) {
	// the directive that we are always comparing to
	originalSchema, err := graphql.LoadSchema(`
		type CatPhoto {
			species: String
		}

		type DogPhoto {
			species: String
		}

		union Photo = CatPhoto | DogPhoto
	`)
	// make sure nothing went wrong
	if !assert.Nil(t, err, "original schema didn't parse") {
		return
	}

	t.Run("Matching", func(t *testing.T) {
		// merge the schema with one that should work
		_, err := testMergeSchemas(originalSchema, `
			type CatPhoto {
				species: String
			}

			type DogPhoto {
				species: String
			}

			union Photo = CatPhoto | DogPhoto
		`)
		if err != nil {
			t.Error(err.Error())
		}
	})

	// the table we are testing
	testMergeRunNegativeTable(t, originalSchema, []testMergeTableRow{
		{
			"Different Subtypes",
			`
				type NotCatPhoto {
					url: String
				}

				type NotDogPhoto {
					url: String
				}

				union Photo = NotCatPhoto | NotDogPhoto
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

	t.Run("Matching", func(t *testing.T) {
		// merge the schema with one that should work
		_, err := testMergeSchemas(originalSchema, `
			interface Foo {
				name: String!
			}
		`)
		if err != nil {
			t.Error(err.Error())
		}
	})

	// the table we are testing
	testMergeRunNegativeTable(t, originalSchema, []testMergeTableRow{
		{
			"Different Field Directives",
			`
				directive @foo on FIELD_DEFINITION

				interface Foo {
					name: String! @foo
				}
			`,
		},
		{
			"Different Field Types",
			`
				interface Foo {
					name: String
				}
			`,
		},
		{
			"Different Fields",
			`
				interface Foo {
					name: String!
					lastName: String!
				}
			`,
		},
		{
			"Different Arguments",
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
	Schema  string
}

func testMergeRunNegativeTable(t *testing.T, original *ast.Schema, table []testMergeTableRow) {
	for _, row := range table {
		t.Run(row.Message, func(t *testing.T) {
			// we're assuming the test needs to fail
			_, err := testMergeSchemas(original, row.Schema)

			if err == nil {
				t.Error("Did not encounter an error when one was expected")
			}
		})
	}
}

func testMergeSchemas(schema1 *ast.Schema, schema2Str string) (*ast.Schema, error) {
	// create a schema with the provided content
	schema2, _ := graphql.LoadSchema(schema2Str)

	// create remote schemas with each
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema1, URL: "url1"},
		{Schema: schema2, URL: "url2"},
	})

	if err != nil {
		return nil, err
	}

	return gateway.schema, err
}
