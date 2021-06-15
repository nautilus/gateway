package gateway

import (
	"errors"
	"testing"

	"github.com/nautilus/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/v2/ast"
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

func TestMergeSchema_inputTypes(t *testing.T) {
	// create the first schema
	originalSchema, err := graphql.LoadSchema(`
		input Foo {
			firstName: String!
		}
	`)
	assert.Nil(t, err)

	t.Run("Matching", func(t *testing.T) {
		// merge the schema with one that should work
		schema, err := testMergeSchemas(t, originalSchema, `
			input Foo {
				firstName: String!
			}
		`)
		if err != nil {
			t.Error(err.Error())
		}

		// look up the Foo input type
		inputType := schema.Types["Foo"]

		if !assert.NotNil(t, inputType, "Could not find input type Foo") {
			return
		}

		if len(inputType.Fields) != 1 {
			t.Errorf("Encountered incorrect number of fields. Expected 1 found %v", len(inputType.Fields))
			return
		}
	})

	// the table we are testing
	testMergeRunNegativeTable(t, []testMergeTableRow{
		{
			"Conflicting Fields",
			`
				input Foo {
					firstName: String!
				}
			`,
			`
				input Foo {
					lastName: String!
				}
			`,
		},
		{
			"Different Fields",
			`
				input Foo {
					firstName: String!
					lastName: String!
				}
			`,
			`
				input Foo {
					lastName: String!
				}
			`,
		},
		{
			"Conflicting directives",
			`
				input Foo {
					lastName: String!
				}
			`,
			`
				directive @foo on INPUT_OBJECT

				input Foo @foo {
					lastName: String!
				}
			`,
		},
		{
			"Conflicting field directives",
			`
				input Foo {
					lastName: String!
				}
			`,
			`
				directive @foo on FIELD_DEFINITION

				input Foo  {
					lastName: String! @foo
				}
			`,
		},
		{
			"Conflicting total field directives",
			`
				directive @foo on FIELD_DEFINITION

				directive @bar on FIELD_DEFINITION

				input Foo {
					lastName: String! @foo @bar
				}
			`,
			`
				directive @foo on FIELD_DEFINITION

				input Foo  {
					lastName: String! @foo
				}
			`,
		},
	})
}

func TestMergeSchema_objectTypes(t *testing.T) {
	t.Run("Merge fields", func(t *testing.T) {
		// create the first schema
		originalSchema, err := graphql.LoadSchema(`
			type User {
				firstName: String!
			}
		`)
		assert.Nil(t, err)

		// merge the schema with one that should work
		schema, err := testMergeSchemas(t, originalSchema, `
				type User {
					lastName: String!
				}
		`)
		if err != nil {
			t.Error(err.Error())
			return
		}

		// look up the definition for the User type
		definition, exists := schema.Types["User"]
		// make sure the definition exists
		assert.True(t, exists)

		assert.Len(t, definition.Fields, 2)

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
	testMergeRunNegativeTable(t, []testMergeTableRow{
		{
			"Conflicting Field Type",
			`
				type User {
					firstName: String
				}
			`,
			`
				type User {
					firstName: Int
				}
			`,
		},
		{
			"Conflicting declaration directives",
			`
				directive @foo(url: String!) on OBJECT

				type User @foo {
					firstName: String
				}
			`,
			`
				type User {
					firstName: String
				}
			`,
		},
		{
			"Conflicting field directives",
			`
				directive @foo(url: String!) on FIELD_DEFINITION

				type User {
					firstName: String! @foo(url: "3")
				}
			`,
			`
				directive @foo(url: String!) on FIELD_DEFINITION

				type User {
					firstName: String! @foo(url: "2")
				}
			`,
		},
		{
			"Conflicting field descriptions",
			`
				type User {
					"description"
					firstName: String!
				}
			`,
			`
				type User {
					"other-description"
					firstName: String!
				}
			`,
		},
		{
			"Conflicting field argument default value",
			`
				type User {
					"description"
					firstName: String!
				}
			`,
			`
				type User {
					"other-description"
					firstName: String!
				}
			`,
		},
		{
			"Conflicting number of directive arguments",
			`
				directive @foo(url: String!, url2: String!) on FIELD_DEFINITION

				type User {
					firstName: String! @foo(url: "3")
				}
			`,
			`
				directive @foo(url: String!, url2: String!) on FIELD_DEFINITION

				type User {
					firstName: String! @foo(url: "3", url2: "3")
				}
			`,
		},
		{
			"Conflicting name of multiple arguments",
			`
				type User {
					firstName(url: String, url2: String): String!
				}
			`,
			`
				type User {
					firstName(url: String, url3: String): String!
				}
			`,
		},
		{
			"Conflicting field types",
			`
				type User {
					firstName: [String]
				}
			`,
			`
				type User {
					firstName: String
				}
			`,
		},
		{
			"Conflicting inner field types",
			`
				type User {
					firstName: [Int]
				}
			`,
			`
				type User {
					firstName: [String]
				}
			`,
		},
	})
}

func TestMergeSchema_enums(t *testing.T) {
	t.Run("Matching", func(t *testing.T) {
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

		// merge the schema with one that should work
		_, err = testMergeSchemas(t, originalSchema, `
			enum Foo {
				Bar
				Baz
			}
		`)
		if err != nil {
			t.Error(err.Error())
		}
	})

	// the table we are testing
	testMergeRunNegativeTable(t, []testMergeTableRow{
		{
			"Conflicting Names",
			`
				enum Foo {
					Bar
					Baz
				}
			`,
			`
				enum Foo {
					Bar
				}
			`,
		},
		{
			"Conflicting value descriptions",
			`
				enum Foo {
					"description"
					Bar
					Baz
				}
			`,
			`
				enum Foo {
					Bar
				}
			`,
		},
		{
			"Conflicting descriptions",
			`
				"description"
				enum Foo {
					Bar
					Baz
				}
			`,
			`
				enum Foo {
					Bar
				}
			`,
		},
	})
}

func TestMergeSchema_directives(t *testing.T) {
	t.Run("Matching", func(t *testing.T) {
		// the directive that we are always comparing to
		originalSchema, err := graphql.LoadSchema(`
			"description"
			directive @foo(url: String = "url") on FIELD_DEFINITION
		`)
		// make sure nothing went wrong
		if !assert.Nil(t, err, "original schema didn't parse") {
			return
		}

		// merge the schema with one that should work
		_, err = testMergeSchemas(t, originalSchema, `
			"description"
			directive @foo(url: String = "url") on FIELD_DEFINITION
		`)
		if err != nil {
			t.Error(err.Error())
		}
	})

	// run the table of tests
	testMergeRunNegativeTable(t, []testMergeTableRow{
		{
			"Different Argument Type",
			`
				directive @foo(url: String) on FIELD_DEFINITION
			`,
			`
				directive @foo(url: String!) on FIELD_DEFINITION
			`,
		},
		{
			"Different Arguments",
			`
				directive @foo(url: String) on FIELD_DEFINITION
			`,
			`
				directive @foo(url: String, number: Int) on FIELD_DEFINITION
			`,
		},
		{
			"Different Location",
			`
				directive @foo on FIELD_DEFINITION
			`,
			`
				directive @foo on FRAGMENT_SPREAD
			`,
		},
		{
			"Different description",
			`
				"other-desription"
				directive @foo on FIELD_DEFINITION
			`,
			`
				"desription"
				directive @foo on FIELD_DEFINITION
			`,
		},
		{
			"Different field types",
			`
				directive @foo(foo: String) on FIELD_DEFINITION
			`,
			`
				"desription"
				directive @foo(foo: [String]) on FIELD_DEFINITION
			`,
		},
		{
			"Different Number of Locations",
			`
				directive @foo on FIELD_DEFINITION
			`,
			`
				directive @foo on FRAGMENT_SPREAD | FIELD_DEFINITION
			`,
		},
		{
			"Different Description",
			`
				"description"
				directive @foo on FIELD_DEFINITION
			`,
			`
				"other description"
				directive @foo on FIELD_DEFINITION
			`,
		},
		{
			"Different Default Value",
			`
				directive @foo(url: String = "url") on FIELD_DEFINITION
			`,
			`
				directive @foo(url: String = "not-url") on FIELD_DEFINITION
			`,
		},
	})
}

func TestMergeSchema_union(t *testing.T) {
	t.Run("Matching", func(t *testing.T) {
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

		// merge the schema with one that should work
		schema, err := testMergeSchemas(t, originalSchema, `
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

		schemaUnion := schema.Types["Photo"]

		previousTypes := Set{}
		for _, subType := range schemaUnion.Types {
			previousTypes.Add(subType)
		}

		assert.True(t, previousTypes["CatPhoto"])
		assert.True(t, previousTypes["DogPhoto"])
	})

	// the table we are testing
	testMergeRunNegativeTable(t, []testMergeTableRow{
		{
			"Different Subtypes",
			`
				type CatPhoto {
					species: String
				}

				type DogPhoto {
					species: String
				}

				union Photo = CatPhoto | DogPhoto
			`,
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
		{
			"Different number of subtypes",
			`
				type CatPhoto {
					species: String
				}

				type DogPhoto {
					species: String
				}

				union Photo = CatPhoto | DogPhoto
			`,
			`
				type CatPhoto {
					url: String
				}

				type DogPhoto {
					url: String
				}

				type LemurPhoto {
					url: String
				}


				union Photo = CatPhoto | DogPhoto | LemurPhoto
			`,
		},
	})
}

func TestMergeSchema_unions(t *testing.T) {
	t.Run("Matching", func(t *testing.T) {
		originalSchema, err := graphql.LoadSchema(`
			type Foo {
				name: String!
			}

			type Bar {
				lastName: String!
			}

			union Foobar = Foo | Bar
		`)
		if !assert.Nil(t, err, "original schema didn't parse") {
			return
		}

		// merge the schema with a compatible schema
		schema, err := testMergeSchemas(t, originalSchema, `
			type Baz {
				name: String!
			}

			type Qux {
				middleName: String!
			}

			union Bazqux = Baz | Qux
		`)
		if err != nil {
			t.Error(err.Error())
			return
		}

		possibleTypes := schema.GetPossibleTypes(schema.Types["Foobar"])
		if len(possibleTypes) != 2 {
			t.Errorf("Union has incorrect number of types. Expected 2, found %v", len(schema.GetPossibleTypes(schema.Types["Foobar"])))
			return
		}

		// keep the unique set of the types we visisted
		visited := Set{}
		for _, possibleType := range possibleTypes {
			visited.Add(possibleType.Name)
		}

		assert.True(t, visited["Foo"], "did not have Bar in possible type")
		assert.True(t, visited["Bar"], "did not have Baz in possible type")

		possibleTypes = schema.GetPossibleTypes(schema.Types["Bazqux"])
		if len(possibleTypes) != 2 {
			t.Errorf("Union has incorrect number of types. Expected 2, found %v", len(schema.GetPossibleTypes(schema.Types["Bazqux"])))
			return
		}

		visited = Set{}
		for _, possibleType := range possibleTypes {
			visited.Add(possibleType.Name)
		}

		assert.True(t, visited["Baz"], "did not have Bar in possible type")
		assert.True(t, visited["Qux"], "did not have Baz in possible type")
	})
}

func TestMergeSchema_interfaces(t *testing.T) {
	t.Run("Matching", func(t *testing.T) {
		// the directive that we are always comparing to
		originalSchema, err := graphql.LoadSchema(`
			interface Foo {
				name: String!
			}

			type User implements Foo {
				name: String!
			}
		`)
		// make sure nothing went wrong
		if !assert.Nil(t, err, "original schema didn't parse") {
			return
		}

		// merge the schema with one that should work
		schema, err := testMergeSchemas(t, originalSchema, `
			interface Foo {
				name: String!
			}

			type NotUser implements Foo {
				name: String!
			}
		`)
		if err != nil {
			t.Error(err.Error())
			return
		}

		possibleTypes := schema.GetPossibleTypes(schema.Types["Foo"])
		// we need to make sure that the interface has 3 possible types: Foo, User, and NotUser
		if len(possibleTypes) != 3 {
			t.Errorf("Interface has incorrect number of types. Expected 3, found %v", len(schema.GetPossibleTypes(schema.Types["Foo"])))
			return
		}

		// keep the unique set of the types we visisted
		visited := Set{}
		for _, possibleType := range possibleTypes {
			visited.Add(possibleType.Name)
		}

		assert.True(t, visited["Foo"], "did not have Foo in possible type")
		assert.True(t, visited["User"], "did not have User in possible type")
		assert.True(t, visited["NotUser"], "did not have NotUser in possible type")

	})

	// the table we are testing
	testMergeRunNegativeTable(t, []testMergeTableRow{
		{
			"Different Field Directives",
			`
				interface Foo {
					name: String!
				}
			`,
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
					name: String!
				}
			`,
			`
				interface Foo {
					name: String
				}
			`,
		},
		{
			"Different Descriptions",
			`
				"description"
				interface Foo {
					name: String!
				}
			`,
			`
				"not-description"
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
				}
			`,
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
					name: String!
				}
			`,
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
	Schema1 string
	Schema2 string
}

func testMergeRunNegativeTable(t *testing.T, table []testMergeTableRow) {
	for _, row := range table {
		t.Run(row.Message, func(t *testing.T) {
			original, _ := graphql.LoadSchema(row.Schema1)
			if original == nil {
				t.Error("could not build schema")
			}

			// we're assuming the test needs to fail
			_, err := testMergeSchemas(t, original, row.Schema2)
			if err == nil {
				t.Log("Did not encounter an error when one was expected")
			}
		})
	}
}

func testMergeSchemas(t *testing.T, schema1 *ast.Schema, schema2Str string) (*ast.Schema, error) {
	// create a schema with the provided content
	schema2, _ := graphql.LoadSchema(schema2Str)
	if schema2 == nil {
		t.Error("could not build schema")
		return nil, errors.New("could not build schema")
	}

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

func TestMergeSchemaDifferentSetsOfInterfaces(t *testing.T) {
	// Thing of schema1 implements one interface
	// Thing of schema2 implements two interfaces

	schema1, err := graphql.LoadSchema(
		//language=GRAPHQL
		`
		type Query {
			node(id: ID!): Node
			Thing(id: ID!): Thing
		}

		interface Node {
			id: ID!
		}
				
		type Thing implements Node {
			id: ID!
			foo: String!
		}
	`)
	assert.Nil(t, err)
	schema2, err := graphql.LoadSchema(
		//language=GRAPHQL
		`
		type Query {
			node(id: ID!): Node
			Thing(id: ID!): Thing
		}
				
		interface Node {
			id: ID!
		}
		
		interface MetaData {
			created_at: String!
		}
		
		type Thing implements Node & MetaData {
			id: ID!
			bar: String!
			created_at: String!
		}
	`)
	assert.Nil(t, err)

	_, err = New([]*graphql.RemoteSchema{
		{Schema: schema2, URL: "url1"},
		{Schema: schema1, URL: "url2"},
	})
	assert.Nil(t, err)
}
