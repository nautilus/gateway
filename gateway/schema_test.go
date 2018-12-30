package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type schemaTableRow struct {
	location string
	query    string
}

func TestSchema_computeFieldLocations(t *testing.T) {
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
		sources = append(sources, RemoteSchema{Schema: schema, Location: source.location})
	}

	locations, err := fieldLocations(sources)
	if err != nil {
		t.Errorf("Encountered error building schema: %s", err.Error())
	}

	allUsersLocation, err := locations.LocationFor("Query", "allUsers")
	assert.Nil(t, err)
	assert.Equal(t, []string{"url1"}, allUsersLocation)

	lastNameLocation, err := locations.LocationFor("User", "lastName")
	assert.Nil(t, err)
	assert.Equal(t, []string{"url1", "url2"}, lastNameLocation)

	firstNameLocation, err := locations.LocationFor("User", "firstName")
	assert.Nil(t, err)
	assert.Equal(t, []string{"url1"}, firstNameLocation)
}
