package gateway

import (
	"strings"
	"testing"

	"github.com/alecaivazis/graphql-gateway/graphql"
	"github.com/stretchr/testify/assert"
)

type schemaTableRow struct {
	location string
	query    string
}

func TestSchema_computeFieldURLs(t *testing.T) {
	schemas := []schemaTableRow{
		{
			"url1",
			`
				type Query {
					allUsers: [User!]!
				}

				type User {
					firstName: String!
					lastName: String!
				}
			`,
		},
		{
			"url2",
			`
				type User {
					lastName: String!
				}
			`,
		},
	}

	// the list of remote schemas
	sources := []*graphql.RemoteSchema{}

	for _, source := range schemas {
		// turn the combo into a remote schema
		schema, _ := graphql.LoadSchema(source.query)

		// add the schema to list of sources
		sources = append(sources, &graphql.RemoteSchema{Schema: schema, URL: source.location})
	}

	locations := fieldURLs(sources, false)

	allUsersURL, err := locations.URLFor("Query", "allUsers")
	assert.Nil(t, err)
	assert.Equal(t, []string{"url1"}, allUsersURL)

	lastNameURL, err := locations.URLFor("User", "lastName")
	assert.Nil(t, err)
	assert.Equal(t, []string{"url1", "url2"}, lastNameURL)

	firstNameURL, err := locations.URLFor("User", "firstName")
	assert.Nil(t, err)
	assert.Equal(t, []string{"url1"}, firstNameURL)

	// make sure we can look up the url for internal
	_, ok := locations["__Schema.types"]
	if !ok {
		t.Error("Could not find internal type")
		return
	}
}

func TestNew_variadicConfiguration(t *testing.T) {

	schemas := []schemaTableRow{
		{
			"url1",
			`
				type Query {
					allUsers: [User!]!
				}

				type User {
					firstName: String!
					lastName: String!
				}
			`,
		},
		{
			"url2",
			`
				type User {
					lastName: String!
				}
			`,
		},
	}

	// the list of remote schemas
	sources := []*graphql.RemoteSchema{}

	for _, source := range schemas {
		// turn the combo into a remote schema
		schema, _ := graphql.LoadSchema(source.query)

		// add the schema to list of sources
		sources = append(sources, &graphql.RemoteSchema{Schema: schema, URL: source.location})
	}

	// create a new schema with the sources and some configuration
	gateway, err := New([]*graphql.RemoteSchema{sources[0]}, func(schema *Gateway) {
		schema.sources = append(schema.sources, sources[1])
	})

	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure that the schema has both sources
	assert.Len(t, gateway.sources, 2)
}

func TestFieldURLs_ignoreIntrospection(t *testing.T) {

	schemas := []schemaTableRow{
		{
			"url1",
			`
				type Query {
					allUsers: [User!]!
				}

				type User {
					firstName: String!
					lastName: String!
				}
			`,
		},
		{
			"url2",
			`
				type User {
					lastName: String!
				}
			`,
		},
	}

	// the list of remote schemas
	sources := []*graphql.RemoteSchema{}

	for _, source := range schemas {
		// turn the combo into a remote schema
		schema, _ := graphql.LoadSchema(source.query)

		// add the schema to list of sources
		sources = append(sources, &graphql.RemoteSchema{Schema: schema, URL: source.location})
	}

	locations := fieldURLs(sources, true)

	for key := range locations {
		if strings.HasPrefix(key, "__") {
			t.Errorf("Found type starting with __: %s", key)
		}
	}
}

func TestFieldURLs_concat(t *testing.T) {
	// create a field url map
	first := FieldURLMap{}
	first.RegisterURL("Parent", "field1", "url1")
	first.RegisterURL("Parent", "field2", "url1")

	// create a second url map
	second := FieldURLMap{}
	second.RegisterURL("Parent", "field2", "url2")
	second.RegisterURL("Parent", "field3", "url2")

	// concatenate the 2
	sum := first.Concat(second)

	// make sure that that there is one entry for Parent.field1
	urlLocations1, err := sum.URLFor("Parent", "field1")
	if err != nil {
		t.Error(err.Error())
		return
	}
	assert.Equal(t, []string{"url1"}, urlLocations1)

	// look up the locations for Parent.field2
	urlLocations2, err := sum.URLFor("Parent", "field2")
	if err != nil {
		t.Error(err.Error())
		return
	}
	assert.Equal(t, []string{"url1", "url2"}, urlLocations2)

	// look up the locations for Parent.field3
	urlLocations3, err := sum.URLFor("Parent", "field3")
	if err != nil {
		t.Error(err.Error())
		return
	}
	assert.Equal(t, []string{"url2"}, urlLocations3)
}

func TestSchemaConfigurator_withPlanner(t *testing.T) {
	schema, _ := graphql.LoadSchema(
		`
			type Query {
				allUsers: [String!]!
			}
		`,
	)

	remoteSchema := &graphql.RemoteSchema{
		Schema: schema,
		URL:    "hello",
	}

	// the planner we will assign
	planner := &MockPlanner{}

	gateway, err := New([]*graphql.RemoteSchema{remoteSchema}, WithPlanner(planner))
	if err != nil {
		t.Error(err.Error())
		return
	}

	assert.Equal(t, planner, gateway.planner)
}
