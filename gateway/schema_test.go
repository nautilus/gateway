package gateway

import (
	"testing"

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
	sources := []RemoteSchema{}

	for _, source := range schemas {
		// turn the combo into a remote schema
		schema, _ := loadSchema(source.query)

		// add the schema to list of sources
		sources = append(sources, RemoteSchema{Schema: schema, URL: source.location})
	}

	locations, err := fieldURLs(sources)
	if err != nil {
		t.Errorf("Encountered error building schema: %s", err.Error())
	}

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
