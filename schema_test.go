package gateway

import (
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

	locations := fieldURLs(sources)

	allUsersURL, err := locations.URLFor("Query", "allUsers")
	assert.Nil(t, err)
	assert.Equal(t, []string{"url1"}, allUsersURL)

	lastNameURL, err := locations.URLFor("User", "lastName")
	assert.Nil(t, err)
	assert.Equal(t, []string{"url1", "url2"}, lastNameURL)

	firstNameURL, err := locations.URLFor("User", "firstName")
	assert.Nil(t, err)
	assert.Equal(t, []string{"url1"}, firstNameURL)
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
	gateway, err := New([]*graphql.RemoteSchema{sources[0]}, func(schema *Schema) {
		schema.sources = append(schema.sources, sources[1])
	})

	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure that the schema has both sources
	assert.Len(t, gateway.sources, 2)
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
