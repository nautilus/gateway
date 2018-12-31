package gateway

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

func TestPlanQuery_singleRootField(t *testing.T) {
	// the location for the schema
	location := "url1"

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "foo", location)

	schema, _ := loadSchema(`
		type Query {
			foo: Boolean
		}
	`)

	// compute the plan for a query that just hits one service
	plans, err := (&NaiveQueryPlanner{}).Plan(`
		{
			foo
		}
	`, schema, locations)
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when building schema: %s", err.Error())
		return
	}

	// the first selection is the only one we care about
	root := plans[0].Steps[0]
	rootField := applyDirectives(root.SelectionSet)[0]

	// make sure that the first step is pointed at the right place
	assert.Equal(t, location, root.URL)

	// we need to be asking for allUsers
	assert.Equal(t, rootField.Name, "foo")

	// there should be anything selected underneath it
	assert.Len(t, rootField.SelectionSet, 0)
}

func TestPlanQuery_singleRootObject(t *testing.T) {
	// the location for the schema
	location := "url1"

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "allUsers", location)
	locations.RegisterURL("User", "firstName", location)
	locations.RegisterURL("User", "friends", location)

	schema, _ := loadSchema(`
		type User {
			firstName: String!
			friends: [User!]!
		}

		type Query {
			allUsers: [User!]!
		}
	`)

	// compute the plan for a query that just hits one service
	selections, err := (&NaiveQueryPlanner{}).Plan(`
		{
			allUsers {
				firstName
				friends {
					firstName
				}
			}
		}
	`, schema, locations)
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when building schema: %s", err.Error())
		return
	}

	// the first selection is the only one we care about
	rootStep := selections[0].Steps[0]
	rootField := applyDirectives(rootStep.SelectionSet)[0]

	// make sure that the first step is pointed at the right place
	assert.Equal(t, location, rootStep.URL)

	// we need to be asking for allUsers
	assert.Equal(t, rootField.Name, "allUsers")

	// grab the field from the top level selection
	field, ok := rootField.SelectionSet[0].(*ast.Field)
	if !ok {
		t.Error("Did not get a field out of the allUsers selection")
	}
	// and from all users we need to ask for their firstName
	assert.Equal(t, "firstName", field.Name)
	assert.Equal(t, "String!", field.Definition.Type.Dump())

	// we also should have asked for the friends object
	friendsField, ok := rootField.SelectionSet[1].(*ast.Field)
	if !ok {
		t.Error("Did not get a friends field out of the allUsers selection")
	}
	// and from all users we need to ask for their firstName
	assert.Equal(t, "friends", friendsField.Name)
	// look at the selection we've made of friends
	subField, ok := friendsField.SelectionSet[0].(*ast.Field)
	if !ok {
		t.Error("Did not get a field out of the allUsers selection")
	}
	assert.Equal(t, "firstName", subField.Name)
}

func TestPlanQuery_subGraphs(t *testing.T) {
	schema, _ := loadSchema(`
		type User {
			firstName: String!
			catPhotos: [CatPhoto!]!
		}

		type CatPhoto {
			URL: String!
		}

		type Query {
			allUsers: [User!]!
		}

	`)

	// the location of the user service
	userLocation := "user-location"
	// the location of the cat service
	catLocation := "cat-location"

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "allUsers", userLocation)
	locations.RegisterURL("User", "firstName", userLocation)
	locations.RegisterURL("User", "catPhotos", catLocation)
	locations.RegisterURL("CatPhoto", "URL", catLocation)

	plans, err := (&NaiveQueryPlanner{}).Plan(`
		{
			allUsers {
				firstName
				catPhotos {
					URL
				}
			}
		}
	`, schema, locations)
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when building schema: %s", err.Error())
		return
	}

	// there are 2 steps of a single plan that we care about
	// the first step is grabbing allUsers and their firstName
	// the second step is grabbing User catPhotos

	// the first step should have all users
	firstStep := plans[0].Steps[0]
	firstField := applyDirectives(firstStep.SelectionSet)[0]
	// it is resolved against the user service
	assert.Equal(t, userLocation, firstStep.URL)

	// make sure it is for allUsers
	assert.Equal(t, firstField.Name, "allUsers")

	// all users should have only one selected value since `catPhotos` is from another service
	if len(firstField.SelectionSet) > 1 {
		for _, selection := range applyDirectives(firstField.SelectionSet) {
			fmt.Println(selection.Name)
		}
		t.Error("Encountered too many fields on allUsers selection set")
		return
	}

	// grab the field from the top level selection
	field, ok := firstField.SelectionSet[0].(*ast.Field)
	if !ok {
		t.Error("Did not get a field out of the allUsers selection")
	}
	// and from all users we need to ask for their firstName
	assert.Equal(t, "firstName", field.Name)
	assert.Equal(t, "String!", field.Definition.Type.Dump())

}

// func TestPlanQuery_multipleRootFields(t *testing.T) {
// 	t.Error("Not implemented")
// }

// func TestPlanQuery_mutationsInSeries(t *testing.T) {
// 	t.Error("Not implemented")
// }

// func TestPlanQuery_siblingFields(t *testing.T) {
// 	t.Error("Not implemented")
// }

// func TestPlanQuery_duplicateFieldsOnEither(t *testing.T) {
// 	// make sure that if I have the same field defined on both schemas we dont create extraneous calls
// 	t.Error("Not implemented")
// }

// func TestPlanQuery_groupsConflictingFields(t *testing.T) {
// 	// if I can find a field in 4 different services, look for the one I"m already going to
// 	t.Error("Not implemented")
// }

// func TestPlanQuery_combineFragments(t *testing.T) {
// 	// fragments could bring in different fields from different services
// 	t.Error("Not implemented")
// }

// func TestPlanQuery_threadVariables(t *testing.T) {
// 	t.Error("Not implemented")
// }
