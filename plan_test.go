package gateway

import (
	"fmt"
	"testing"

	"github.com/nautilus/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

func TestPlanQuery_singleRootField(t *testing.T) {
	// the location for the schema
	location := "url1"

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "foo", location)

	schema, _ := graphql.LoadSchema(`
		type Query {
			foo: Boolean
		}
	`)

	// compute the plan for a query that just hits one service
	plans, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
		Query:     "{ foo }",
		Schema:    schema,
		Locations: locations,
	})
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when building schema: %s", err.Error())
		return
	}

	// the first selection is the only one we care about
	root := plans[0].RootStep.Then[0]
	// there should only be one selection
	if len(root.SelectionSet) != 1 {
		t.Error("encountered the wrong number of selections under root step")
		return
	}
	rootField := graphql.SelectedFields(root.SelectionSet)[0]

	// make sure that the first step is pointed at the right place
	queryer := root.Queryer.(*graphql.SingleRequestQueryer)
	assert.Equal(t, location, queryer.URL())

	// we need to be asking for Query.foo
	assert.Equal(t, rootField.Name, "foo")

	// there should be anything selected underneath it
	assert.Len(t, rootField.SelectionSet, 0)
}

func TestPlanQuery_includeFragmentsSameLocation(t *testing.T) {
	// the location for the schema
	location := "url1"

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "foo", location)

	schema, _ := graphql.LoadSchema(`
		type Query {
			foo: Boolean
		}
	`)

	// compute the plan for a query that just hits one service
	plans, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
		Query: `
			query MyQuery {
				...Foo
			}

			fragment Foo on Query {
				foo
			}
		`,
		Schema:    schema,
		Locations: locations,
	})
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when planning query: %s", err.Error())
		return
	}

	if len(plans[0].RootStep.Then) != 1 {
		t.Error("Could not find the step with fragment spread")
		return
	}

	// the first selection is the only one we care about
	root := plans[0].RootStep.Then[0]

	// there should only be one selection
	if len(root.SelectionSet) != 1 {
		t.Errorf("encountered the wrong number of selections under root step: %v", len(root.SelectionSet))
		return
	}

	// there should be a single selection that is a spread of the fragment Foo
	fragment, ok := root.SelectionSet[0].(*ast.FragmentSpread)
	if !ok {
		t.Error("Root selection was not a fragment spread", root.SelectionSet[0])
		return
	}

	// make sure that the fragment has the right name
	assert.Equal(t, "Foo", fragment.Name)

	// we need to make sure that the fragment definition matches expectation
	fragmentDef := root.QueryDocument.Fragments.ForName("Foo")
	if fragmentDef == nil {
		t.Error("Could not find fragment definition for Foo")
		return
	}

	// there should only be one selection in the fragment
	if len(fragmentDef.SelectionSet) != 1 {
		t.Errorf("Encountered the incorrect number of fields under fragment definition")
		return
	}

	// we should have selected foo
	assert.Equal(t, "foo", graphql.SelectedFields(fragmentDef.SelectionSet)[0].Name)
}

// Tests that location selection for Fields within Fragment Spreads are correctly
// prioritized, to avoid unnecessary federation steps.
func TestPlanQuery_includeFragmentsBoundaryTypes(t *testing.T) {
	// the locations for the schema
	location1 := "url1"
	location2 := "url2"

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "foo", location1)
	locations.RegisterURL("BoundaryType", "a", location2)
	locations.RegisterURL("BoundaryType", "a", location1)
	locations.RegisterURL("BoundaryType", "b", location2)
	locations.RegisterURL("BoundaryType", "b", location1)
	locations.RegisterURL("boundaryA", "fieldA", location2)
	locations.RegisterURL("boundaryA", "fieldA", location1)
	locations.RegisterURL("boundaryB", "fieldB", location2)
	locations.RegisterURL("boundaryB", "fieldB", location1)

	schema, _ := graphql.LoadSchema(`
		type Query {
			foo: BoundaryType!	
		}

		type BoundaryType {
			a: boundaryA!
			b: boundaryB!
		}

		type boundaryA {
			fieldA: String!
		}

		type boundaryB {
			fieldB: String!
		}
	`)

	plans, err := (&MinQueriesPlanner{}).Plan(
		&PlanningContext{
			Query: `
				query {
					foo {
						...Foo
					}
				}

				fragment Foo on BoundaryType {
					a {
						... aFragment
					}
					b {
						... bFragment
					}
				}

				fragment aFragment on boundaryA {
					fieldA
				}

				fragment bFragment on boundaryB {
					fieldB
				}
			`,
			Schema:    schema,
			Locations: locations,
		},
	)
	if err != nil {
		t.Errorf("encountered error when planning query: %s", err.Error())
		return
	}

	assert.Equal(t, len(plans), 1)
	assert.Equal(t, len(plans[0].RootStep.Then), 1, "Expected only one child step, got: %d", len(plans[0].RootStep.Then))
	assert.Equal(t, len(plans[0].RootStep.Then[0].Then), 0, "Expected no children steps, got: %d", len(plans[0].RootStep.Then[0].Then))
}

func TestPlanQuery_includeFragmentsDifferentLocation(t *testing.T) {
	// the locations for the schema
	location1 := "url1"
	location2 := "url2"

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "foo", location1)
	locations.RegisterURL("Query", "bar", location2)

	schema, _ := graphql.LoadSchema(`
		type Query {
			foo: Boolean
			bar: Boolean
		}
	`)

	// compute the plan for a query that just hits one service
	plans, err := (&MinQueriesPlanner{}).Plan(
		&PlanningContext{
			Query: `
				query MyQuery {
					...Foo
				}

				fragment Foo on Query {
					foo
					bar
				}
			`,
			Schema:    schema,
			Locations: locations,
		})
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when planning query: %s", err.Error())
		return
	}

	if len(plans[0].RootStep.Then) != 2 {
		t.Errorf("Encountered incorrect number of steps after root step. Expected 2, found %v", len(plans[0].RootStep.Then))
		return
	}

	// get the step for location 1
	location1Step := plans[0].RootStep.Then[0]
	// make sure that the step has only one selection (the fragment)
	if len(location1Step.SelectionSet) != 1 {
		t.Errorf("Encountered incorrect number of selections under location 1 step. Expected 1, found %v", len(location1Step.SelectionSet))
		return
	}
	assert.Equal(t, &ast.FragmentSpread{Name: "Foo"}, location1Step.SelectionSet[0])

	// get the step for location 2
	location2Step := plans[0].RootStep.Then[1]
	// make sure that the step has only one selection (the fragment)
	if len(location2Step.SelectionSet) != 1 {
		t.Errorf("Encountered incorrect number of selections under location 2 step. Expected 1, found %v", len(location2Step.SelectionSet))
		return
	}
	assert.Equal(t, &ast.FragmentSpread{Name: "Foo"}, location2Step.SelectionSet[0])

	// we also should have a definition for the fragment that only includes the fields to location 1
	location1Defn := location1Step.FragmentDefinitions[0]
	location2Defn := location2Step.FragmentDefinitions[0]

	encounteredFields := Set{}

	for _, definition := range (ast.FragmentDefinitionList{location1Defn, location2Defn}) {
		assert.Equal(t, "Query", definition.TypeCondition)
		assert.Equal(t, "Foo", definition.Name)
		if len(definition.SelectionSet) != 1 {
			t.Errorf("Encountered incorrect number of selections under fragment definition for location 1. Expected 1 found %v", len(location1Defn.SelectionSet))
			return
		}

		// add the field we encountered to the set
		encounteredFields.Add(graphql.SelectedFields(definition.SelectionSet)[0].Name)
	}

	// make sure we saw both the step for "foo" and the step for "bar"
	if !encounteredFields.Has("foo") && !encounteredFields.Has("bar") {
		t.Error("Did not encounter both foo and bar steps")
		return
	}
}

func TestPlanQuery_includeInlineFragments(t *testing.T) {
	// the locations for the schema
	location1 := "url1"
	location2 := "url2"

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "foo", location1)
	locations.RegisterURL("Query", "bar", location2)

	schema, _ := graphql.LoadSchema(`
		type Query {
			foo: Boolean
			bar: Boolean
		}
	`)

	// compute the plan for a query that just hits one service
	plans, err := (&MinQueriesPlanner{}).Plan(
		&PlanningContext{
			Query: `
				query MyQuery {
					... on Query {
						foo
						bar
					}
				}
			`,
			Schema:    schema,
			Locations: locations,
		})
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when planning query: %s", err.Error())
		return
	}

	if len(plans[0].RootStep.Then) != 2 {
		t.Errorf("Encountered incorrect number of steps after root step. Expected 2, found %v", len(plans[0].RootStep.Then))
		return
	}

	// get the step for location 1
	location1Step := plans[0].RootStep.Then[0]
	assert.Equal(t, []string{}, location1Step.InsertionPoint)
	// make sure that the step has only one selection (the fragment)
	if len(location1Step.SelectionSet) != 1 {
		t.Errorf("Encountered incorrect number of selections under location 1 step. Expected 1, found %v", len(location1Step.SelectionSet))
		return
	}

	// get the step for location 2
	location2Step := plans[0].RootStep.Then[1]
	assert.Equal(t, []string{}, location2Step.InsertionPoint)
	// make sure that the step has only one selection (the fragment)
	if len(location2Step.SelectionSet) != 1 {
		t.Errorf("Encountered incorrect number of selections under location 2 step. Expected 1, found %v", len(location2Step.SelectionSet))
		return
	}

	// we also should have a definition for the fragment that only includes the fields to location 1
	location1Defn := location1Step.SelectionSet[0]
	location2Defn := location2Step.SelectionSet[0]

	encounteredFields := Set{}

	for _, definition := range (ast.SelectionSet{location1Defn, location2Defn}) {
		fragment, ok := definition.(*ast.InlineFragment)
		if !ok {
			t.Error("Did not encounter an inline fragment")
			return
		}

		assert.Equal(t, "Query", fragment.TypeCondition)

		if len(fragment.SelectionSet) != 1 {
			t.Errorf("Encountered incorrect number of selections under fragment definition. Expected 1 found %v", len(fragment.SelectionSet))
			return
		}

		// add the field we encountered to the set
		encounteredFields.Add(graphql.SelectedFields(fragment.SelectionSet)[0].Name)
	}

	// make sure we saw both the step for "foo" and the step for "bar"
	if !encounteredFields.Has("foo") && !encounteredFields.Has("bar") {
		t.Error("Did not encounter both foo and bar steps")
		return
	}
}

func TestPlanQuery_nestedInlineFragmentsSameLocation(t *testing.T) {
	// the locations for the schema
	loc1 := "url1"
	loc2 := "url2"

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "foo", loc1)
	locations.RegisterURL("Query", "bar", loc2)

	schema, _ := graphql.LoadSchema(`
		type Query {
			foo: Boolean
			bar: Boolean
		}
	`)

	plans, err := (&MinQueriesPlanner{}).Plan(
		&PlanningContext{
			Query: `
				query MyQuery {
					... on Query {
						... on Query {
							foo
						}
						bar
					}
				}
			`,
			Schema:    schema,
			Locations: locations,
		})
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when planning query: %s", err.Error())
		return
	}

	// grab the 2 sibling steps
	steps := plans[0].RootStep.Then
	if !assert.Len(t, steps, 2) {
		return
	}

	var loc1Step *QueryPlanStep
	var loc2Step *QueryPlanStep

	// find the steps
	for _, step := range steps {
		// look at the queryer to figure out where the request is going
		if queryer, ok := step.Queryer.(*graphql.SingleRequestQueryer); ok {
			if queryer.URL() == loc1 {
				loc1Step = step
			} else if queryer.URL() == loc2 {
				loc2Step = step
			}
		} else {
			t.Error("Encountered non-network queryer")
			return
		}
	}

	// the step that's going to location 1 should be equivalent to
	// query MyQuery {
	// 		... on Query {
	// 			... on Query {
	// 				foo
	// 			}
	//		}
	// }

	// that first slection should be an inline fragment
	assert.NotNil(t, loc1Step)
	if !assert.Len(t, loc1Step.SelectionSet, 1) {
		return
	}
	loc1Selection, ok := loc1Step.SelectionSet[0].(*ast.InlineFragment)
	if !assert.True(t, ok) {
		return
	}

	// there should be one selection in that inline fragment
	if !assert.Len(t, loc1Selection.SelectionSet, 1) {
		return
	}
	loc1SubSelection, ok := loc1Selection.SelectionSet[0].(*ast.InlineFragment)
	if !assert.True(t, ok, "first sub-selection in location 1 selection is not an inline fragment: \n%v", log.FormatSelectionSet(loc1Selection.SelectionSet)) {
		return
	}

	// there should be one field
	if !assert.Len(t, loc1SubSelection.SelectionSet, 1) {
		return
	}
	loc1Field, ok := loc1SubSelection.SelectionSet[0].(*ast.Field)
	if !assert.True(t, ok) {
		return
	}

	// it should be for the field "foo"
	assert.Equal(t, "foo", loc1Field.Name)

	// the step that's going to location 2 should be equivalent to
	// query MyQuery {
	// 		... on Query {
	// 			bar
	// 		}
	// }

	if !assert.Len(t, loc2Step.SelectionSet, 1) {
		return
	}
	loc2Selection, ok := loc2Step.SelectionSet[0].(*ast.InlineFragment)
	if !assert.True(t, ok) {
		return
	}

	// it should have one selection that's a field
	if !assert.Len(t, loc2Selection.SelectionSet, 1) {
		return
	}
	loc2Field, ok := loc2Selection.SelectionSet[0].(*ast.Field)
	if !assert.True(t, ok) {
		return
	}

	assert.Equal(t, "bar", loc2Field.Name)
}

func TestPlanQuery_singleRootObject(t *testing.T) {
	// the location for the schema
	location := "url1"

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "allUsers", location)
	locations.RegisterURL("User", "firstName", location)
	locations.RegisterURL("User", "friends", location)

	schema, _ := graphql.LoadSchema(`
		type User {
			firstName: String!
			friends: [User!]!
		}

		type Query {
			allUsers: [User!]!
		}
	`)

	// compute the plan for a query that just hits one service
	selections, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
		Query: `
			{
				allUsers {
					firstName
					friends {
						firstName
						friends {
							firstName
						}
					}
				}
			}
		`,
		Schema:    schema,
		Locations: locations,
	})
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when building schema: %s", err.Error())
		return
	}

	// the first selection is the only one we care about
	rootStep := selections[0].RootStep.Then[0]

	// there should only be one selection
	if len(rootStep.SelectionSet) != 1 {
		t.Error("encountered the wrong number of selections under root step")
		return
	}

	rootField := graphql.SelectedFields(rootStep.SelectionSet)[0]

	// make sure that the first step is pointed at the right place
	queryer := rootStep.Queryer.(*graphql.SingleRequestQueryer)
	assert.Equal(t, location, queryer.URL())

	// we need to be asking for allUsers
	assert.Equal(t, rootField.Name, "allUsers")

	// grab the field from the top level selection
	field, ok := rootField.SelectionSet[0].(*ast.Field)
	if !ok {
		t.Error("Did not get a field out of the allUsers selection")
		return
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
	firstNameField, ok := friendsField.SelectionSet[0].(*ast.Field)
	if !ok {
		t.Error("Did not get a field out of the allUsers selection")
	}
	assert.Equal(t, "firstName", firstNameField.Name)

	// there should be a second field for friends
	friendsInnerField, ok := friendsField.SelectionSet[1].(*ast.Field)
	if !ok {
		t.Error("Did not get an  inner friends out of the allUsers selection")
	}
	assert.Equal(t, "friends", friendsInnerField.Name)

	// and a field below it for their firstName
	firstNameInnerField, ok := friendsInnerField.SelectionSet[0].(*ast.Field)
	if !ok {
		t.Error("Did not get an  inner firstName out of the allUsers selection")
	}
	assert.Equal(t, "firstName", firstNameInnerField.Name)

}

func TestPlanQuery_subGraphs(t *testing.T) {
	schema, _ := graphql.LoadSchema(`
		type User {
			firstName: String!
			catPhotos: [CatPhoto!]!
		}

		type CatPhoto {
			URL: String!
			owner: User!
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
	locations.RegisterURL("CatPhoto", "owner", userLocation)

	plans, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
		Query: `
			{
				allUsers {
					firstName
					catPhotos {
						URL
						owner {
							firstName
						}
					}
				}
			}
		`,
		Schema:    schema,
		Locations: locations,
	})
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when building schema: %s", err.Error())
		return
	}

	// there are 3 steps of a single plan that we care about
	// the first step is grabbing allUsers and their firstName from the user service
	// the second step is grabbing User catPhotos from the cat service
	// the third step is grabb CatPhoto.owner.firstName from the user service from the user service

	// the first step should have all users
	firstStep := plans[0].RootStep.Then[0]
	// make sure we are grabbing values off of Query since its the root
	assert.Equal(t, "Query", firstStep.ParentType)

	// make sure there's a selection set
	if len(firstStep.SelectionSet) != 1 {
		t.Error("first step did not have a selection set")
		return
	}
	firstField := graphql.SelectedFields(firstStep.SelectionSet)[0]
	// it is resolved against the user service
	queryer := firstStep.Queryer.(*graphql.SingleRequestQueryer)
	assert.Equal(t, userLocation, queryer.URL())

	// make sure it is for allUsers
	assert.Equal(t, "allUsers", firstField.Name)

	// all users should have one selected value since `catPhotos` is from another service
	// there will also be an `id` added so that the query can be stitched together
	if len(firstField.SelectionSet) != 2 {
		for _, selection := range graphql.SelectedFields(firstField.SelectionSet) {
			fmt.Println(selection.Name)
		}
		t.Error("Encountered incorrext number of fields on allUsers selection set")
		return
	}

	// grab the field from the top level selection
	field, ok := firstField.SelectionSet[0].(*ast.Field)
	if !ok {
		t.Error("Did not get a field out of the allUsers selection")
		return
	}
	// and from all users we need to ask for their firstName
	assert.Equal(t, "firstName", field.Name)
	assert.Equal(t, "String!", field.Definition.Type.Dump())

	// the second step should ask for the cat photo fields
	if len(firstStep.Then) != 1 {
		t.Errorf("Encountered the wrong number of steps after the first one %v", len(firstStep.Then))
		return
	}
	secondStep := firstStep.Then[0]
	// make sure we will insert the step in the right place
	assert.Equal(t, []string{"allUsers"}, secondStep.InsertionPoint)

	// make sure we are grabbing values off of User since we asked for User.catPhotos
	assert.Equal(t, "User", secondStep.ParentType)
	// we should be going to the catePhoto servie
	queryer = secondStep.Queryer.(*graphql.SingleRequestQueryer)
	assert.Equal(t, catLocation, queryer.URL())
	// we should only want one field selected
	if len(secondStep.SelectionSet) != 1 {
		t.Errorf("Did not have the right number of subfields of User.catPhotos: %v", len(secondStep.SelectionSet))
		return
	}

	// make sure we selected the catPhotos field
	selectedSecondField := graphql.SelectedFields(secondStep.SelectionSet)[0]
	assert.Equal(t, "catPhotos", selectedSecondField.Name)

	// we should have also asked for one field underneath
	secondSubSelection := graphql.SelectedFields(selectedSecondField.SelectionSet)
	if len(secondSubSelection) != 2 {
		t.Error("Encountered the incorrect number of fields selected under User.catPhotos")
	}
	secondSubSelectionField := secondSubSelection[0]
	assert.Equal(t, "URL", secondSubSelectionField.Name)

	// the third step should ask for the User.firstName
	if len(secondStep.Then) != 1 {
		t.Errorf("Encountered the wrong number of steps after the second one %v", len(secondStep.Then))
		return
	}
	thirdStep := secondStep.Then[0]
	// make sure we will insert the step in the right place
	assert.Equal(t, []string{"allUsers", "catPhotos"}, thirdStep.InsertionPoint)

	// make sure we are grabbing values off of User since we asked for User.catPhotos
	assert.Equal(t, "CatPhoto", thirdStep.ParentType)
	// we should be going to the catePhoto service
	queryer = thirdStep.Queryer.(*graphql.SingleRequestQueryer)
	assert.Equal(t, userLocation, queryer.URL())
	// make sure we will insert the step in the right place
	assert.Equal(t, []string{"allUsers", "catPhotos"}, thirdStep.InsertionPoint)

	// we should only want one field selected
	if len(thirdStep.SelectionSet) != 1 {
		t.Errorf("Did not have the right number of subfields of User.catPhotos: %v", len(thirdStep.SelectionSet))
		return
	}

	// make sure we selected the catPhotos field
	selectedThirdField := graphql.SelectedFields(thirdStep.SelectionSet)[0]
	assert.Equal(t, "owner", selectedThirdField.Name)

	// we should have also asked for one field underneath
	thirdSubSelection := graphql.SelectedFields(selectedThirdField.SelectionSet)
	if len(thirdSubSelection) != 1 {
		t.Error("Encountered the incorrect number of fields selected under User.catPhotos")
	}
	thirdSubSelectionField := thirdSubSelection[0]
	assert.Equal(t, "firstName", thirdSubSelectionField.Name)
}

func TestPlanQuery_preferParentLocation(t *testing.T) {

	schema, _ := graphql.LoadSchema(`
		type User {
			id: ID!
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
	// add the
	locations.RegisterURL("User", "id", catLocation)
	locations.RegisterURL("User", "id", userLocation)

	plans, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
		Query: `
			{
				allUsers {
					id
				}
			}
		`,
		Schema:    schema,
		Locations: locations,
	})
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when building schema: %s", err.Error())
		return
	}

	// there should only be 1 step to this query

	// the first step should have all users
	firstStep := plans[0].RootStep.Then[0]
	// make sure we are grabbing values off of Query since its the root
	assert.Equal(t, "Query", firstStep.ParentType)

	// make sure there's a selection set
	if len(firstStep.Then) != 0 {
		t.Errorf("There shouldn't be any dependent step on this one. Found %v.", len(firstStep.Then))
		return
	}
}

func TestPlanQuery_scrubFields(t *testing.T) {
	schema, _ := graphql.LoadSchema(`
		type User {
			id: ID!
			firstName: String!
			favoriteCatSpecies: String!
			catPhotos: [CatPhoto!]!
		}

		type CatPhoto {
			URL: String!
			owner: User!
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
	locations.RegisterURL("CatPhoto", "owner", userLocation)
	locations.RegisterURL("User", "firstName", userLocation)
	locations.RegisterURL("User", "favoriteCatSpecies", catLocation)
	locations.RegisterURL("User", "catPhotos", catLocation)
	locations.RegisterURL("User", "id", catLocation, userLocation)
	locations.RegisterURL("CatPhoto", "URL", catLocation)

	t.Run("Multiple Step Scrubbing", func(t *testing.T) {
		plans, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
			Query: `
				{
					allUsers {
						catPhotos {
							owner {
								firstName
							}
						}
					}
				}
			`,
			Schema:    schema,
			Locations: locations,
		})
		if err != nil {
			t.Error(err.Error())
			return
		}

		// each transition between step requires an id field. None of them were requested so we should have two
		// places where we want to scrub it
		assert.Equal(t, map[string][][]string{
			"id": {
				{"allUsers"},
				{"allUsers", "catPhotos"},
			},
		}, plans[0].FieldsToScrub)
	})

	t.Run("Single Step no Scrubbing", func(t *testing.T) {
		plans, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
			Query: `
					{
						allUsers {
							firstName
						}
					}
			`,
			Schema:    schema,
			Locations: locations,
		})
		if err != nil {
			t.Error(err.Error())
			return
		}

		// each transition between step requires an id field. None of them were requested so we should have two
		// places where we want to scrub it
		assert.Equal(t, map[string][][]string{
			"id": {},
		}, plans[0].FieldsToScrub)
	})

	t.Run("Existing id", func(t *testing.T) {
		plans, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
			Query: `
				{
					allUsers {
						id
						catPhotos {
							owner {
								firstName
							}
						}
					}
				}
			`,
			Schema:    schema,
			Locations: locations,
		})
		if err != nil {
			t.Error(err.Error())
			return
		}

		// each transition between step requires an id field. None of them were requested so we should have two
		// places where we want to scrub it
		assert.Equal(t, map[string][][]string{
			"id": {
				{"allUsers", "catPhotos"},
			},
		}, plans[0].FieldsToScrub)
	})
}

func TestPlanQuery_groupSiblings(t *testing.T) {
	schema, _ := graphql.LoadSchema(`
		type User {
			favoriteCatSpecies: String!
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
	locations.RegisterURL("User", "favoriteCatSpecies", catLocation)
	locations.RegisterURL("User", "catPhotos", catLocation)
	locations.RegisterURL("CatPhoto", "URL", catLocation)

	plans, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
		Query: `
			{
				allUsers {
					favoriteCatSpecies
					catPhotos {
						URL
					}
				}
			}
		`,
		Schema:    schema,
		Locations: locations,
	})
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when building schema: %s", err.Error())
		return
	}

	// there should be 2 steps to this plan.
	// the first queries Query.allUsers
	// the second queries User.favoriteCatSpecies and User.catPhotos

	// the first step should have all users
	firstStep := plans[0].RootStep.Then[0]
	// make sure we are grabbing values off of Query since its the root
	assert.Equal(t, "Query", firstStep.ParentType)

	// make sure there's a selection set
	if len(firstStep.Then) != 1 {
		t.Errorf("Encountered incorrect number of dependent steps on root. Expected 1 found %v", len(firstStep.Then))
		return
	}
}

func TestPlanQuery_nodeField(t *testing.T) {
	// the query to test
	// query {
	//     node(id: $id) {
	//     		... on User {
	//				firstName
	//				lastName
	//    		}
	//     }
	// }

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("User", "firstName", "url1")
	locations.RegisterURL("User", "lastName", "url2")
	locations.RegisterURL("Query", "node", "url1", "url2", internalSchemaLocation)

	// load the query we're going to query
	schema, err := graphql.LoadSchema(`
		interface Node {
			id: ID!
		}

		type User implements Node {
			id: ID!
			firstName: String!
			lastName: String!
		}

		type Query {
			node(id: ID!): Node
		}
	`)
	if err != nil {
		t.Error(err.Error())
	}

	// plan the query
	plans, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
		Query: `
			query($id: ID!) {
				node(id: $id) {
					... on User {
						firstName
						lastName
					}
				}
			}
		`,
		Schema:    schema,
		Locations: locations,
	})
	if err != nil {
		t.Error(err.Error())
		return
	}

	// we should return only one plan
	if !assert.Len(t, plans, 1) {
		return
	}

	// this plan should have 1 step that should hit the internal API
	if !assert.Len(t, plans[0].RootStep.Then, 1, "incorrect number of steps in plan") ||
		!assert.IsType(t, &Gateway{}, plans[0].RootStep.Then[0].Queryer, "first step does not go to the internal API") {
		return
	}
	internalStep := plans[0].RootStep.Then[0]

	// the step should have 2 after it
	if !assert.Len(t, internalStep.Then, 2) {
		return
	}

	// grab the 2 steps
	var url1Step *QueryPlanStep
	var url2Step *QueryPlanStep
	for _, step := range internalStep.Then {
		if queryer, ok := step.Queryer.(*graphql.SingleRequestQueryer); ok && queryer.URL() == "url1" {
			url1Step = step
		} else {
			url2Step = step
		}
	}
	if !assert.NotNil(t, url1Step) || !assert.NotNil(t, url2Step) {
		return
	}

	t.Run("Url1 Step", func(t *testing.T) {
		// the url1 step should have Node as the parent type
		assert.Equal(t, "Node", url1Step.ParentType)
		// there should be one selection set
		if !assert.Len(t, url1Step.SelectionSet, 1) {
			return
		}

		// it should be an inline fragment on User
		inlineFragment, ok := url1Step.SelectionSet[0].(*ast.InlineFragment)
		if !assert.True(t, ok) {
			return
		}
		assert.Equal(t, "User", inlineFragment.TypeCondition)

		// with one selection set: firstName
		if !assert.Len(t, inlineFragment.SelectionSet, 1) {
			return
		}
		assert.Equal(t, "firstName", graphql.SelectedFields(inlineFragment.SelectionSet)[0].Name)
	})

	t.Run("Url2 Step", func(t *testing.T) {
		// the url1 step should have Node as the parent type
		assert.Equal(t, "Node", url2Step.ParentType)
		// there should be one selection set
		if !assert.Len(t, url2Step.SelectionSet, 1) {
			return
		}

		// it should be an inline fragment on User
		inlineFragment, ok := url2Step.SelectionSet[0].(*ast.InlineFragment)
		if !assert.True(t, ok) {
			return
		}
		assert.Equal(t, "User", inlineFragment.TypeCondition)

		// with one selection set: firstName
		if !assert.Len(t, inlineFragment.SelectionSet, 1) {
			return
		}
		assert.Equal(t, "lastName", graphql.SelectedFields(inlineFragment.SelectionSet)[0].Name)
	})
}

func TestPlanQuery_stepVariables(t *testing.T) {
	// the query to test
	// query($id: ID!, $category: String!) {
	// 		user(id: $id) {
	// 			favoriteCatPhoto(category: $category) {
	// 				URL
	// 			}
	// 		}
	// }
	//
	// it should result in one query that depends on $id and the second one
	// which requires $category and $id

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "user", "url1")
	locations.RegisterURL("User", "favoriteCatPhoto", "url2")
	locations.RegisterURL("CatPhoto", "URL", "url2")

	schema, _ := graphql.LoadSchema(`
		type User {
			favoriteCatPhoto(category: String!, owner: ID!): CatPhoto!
		}

		type CatPhoto {
			URL: String!
		}

		type Query {
			user(id: ID!): User
		}
	`)

	// compute the plan for a query that just hits one service
	plans, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
		Query: `
			query($id: ID!, $category: String!) {
				user(id: $id) {
					favoriteCatPhoto(category: $category, owner:$id) {
						URL
					}
				}
			}
		`,
		Schema:    schema,
		Locations: locations,
	})
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when building schema: %s", err.Error())
		return
	}

	// there is only one step
	firstStep := plans[0].RootStep.Then[0]
	// make sure it has the right variable dependencies
	assert.Equal(t, Set{"id": true}, firstStep.Variables)

	// there is a step after
	nextStep := firstStep.Then[0]
	// make sure it has the right variable dependencies
	assert.Equal(t, Set{"category": true, "id": true}, nextStep.Variables)

	if len(nextStep.QueryDocument.Operations) == 0 {
		t.Error("Could not find query document")
		return
	}
	// we need to have a query with id and category since id is passed to node
	if len(nextStep.QueryDocument.Operations[0].VariableDefinitions) != 2 {
		t.Errorf("Did not find the right number of variable definitions in the next step. Expected 2 found %v", len(nextStep.QueryDocument.Operations[0].VariableDefinitions))
		return
	}

	for _, definition := range nextStep.QueryDocument.Operations[0].VariableDefinitions {
		if definition.Variable != "id" && definition.Variable != "category" {
			t.Errorf("Encountered a variable with an unknown name: %v", definition.Variable)
			return
		}
	}
}

func TestPlanQuery_singleFragmentMultipleLocations(t *testing.T) {
	// the locations for the schema
	loc1 := "url1"
	loc2 := "url2"

	// the location map for fields for this query
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "user", loc2)
	locations.RegisterURL("User", "lastName", loc1)
	locations.RegisterURL("User", "id", loc1, loc2)

	schema, _ := graphql.LoadSchema(`
		type User {
			lastName: String
		}

		type Query {
			user: User
		}
	`)

	plans, err := (&MinQueriesPlanner{}).Plan(&PlanningContext{
		Query: `
		query MyQuery {
			...QueryFragment
		}

		fragment QueryFragment on Query {
			user {
				lastName
				...UserInfo
			}
		}

		fragment UserInfo on User {
			lastName
		}
	`,
		Schema:    schema,
		Locations: locations,
	})
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when planning query: %s", err.Error())
		return
	}

	// there is only one direct step
	steps := plans[0].RootStep.Then
	if !assert.Len(t, steps, 1) {
		return
	}

	// there are 2 total steps to this query
	// - the first should be inserted at []:
	//
	// 	query MyQuery {
	// 		...QueryFragment
	// 	}

	// 	fragment QueryFragment on Query {
	// 		user {
	//			id
	// 		}
	// 	}
	//
	//
	// - the second one should be inserted at ["user"]
	//
	// 	query {
	// 		...QueryFragment
	// 	}
	//
	// 	fragment QueryFragment on User {
	//		lastName
	// 		...UserInfo
	// 	}
	//
	// 	fragment UserInfo {
	// 		lastName
	// 	}

	// check the first step
	firstStep := steps[0]

	// there should be one selection on the step
	if !assert.Len(t, firstStep.SelectionSet, 1, "First step had the wrong number of selections") {
		return
	}
	// it should be a fragment spread
	firstSelection, ok := firstStep.SelectionSet[0].(*ast.FragmentSpread)
	if !ok {
		t.Error("First selection step 1 was not a fragment spread")
		return
	}

	// it should be a spread for the Fragment fragment
	assert.Equal(t, "QueryFragment", firstSelection.Name)

	// the definition for QueryFragment should have 1 selections
	queryFragmentDefn := firstStep.FragmentDefinitions.ForName("QueryFragment")
	if !assert.NotNil(t, queryFragmentDefn, "Could not find QueryFragment definition") ||
		!assert.Len(t, queryFragmentDefn.SelectionSet, 1, "Fragment Definition has incorrect number of selections") {
		return
	}
	queryFragmentSelection, ok := queryFragmentDefn.SelectionSet[0].(*ast.Field)
	if !assert.True(t, ok, "query fragment selection was not a field") {
		return
	}

	assert.Equal(t, "user", queryFragmentSelection.Name)
	if !assert.Len(t, queryFragmentSelection.SelectionSet, 1) || !assert.Equal(t, "id", queryFragmentSelection.SelectionSet[0].(*ast.Field).Name) {
		return
	}

	// check the second step
	secondStep := firstStep.Then[0]
	// sanity check the second step meta data
	assert.Equal(t, "User", secondStep.ParentType)
	assert.Equal(t, []string{"user"}, secondStep.InsertionPoint)
	assert.Len(t, secondStep.Then, 0)

	// there should be one selection on the step
	if !assert.Len(t, secondStep.SelectionSet, 1,
		"Second step had the wrong number of selections %v", log.FormatSelectionSet(secondStep.SelectionSet),
	) {
		return
	}

	// it should be a fragment spread
	secondSelection, ok := secondStep.SelectionSet[0].(*ast.FragmentSpread)
	if !assert.True(t, ok, "Second selection is not a fragment spread") {
		return
	}
	assert.Equal(t, "QueryFragment", secondSelection.Name)
	// look up the definition for the step
	defn := secondStep.FragmentDefinitions.ForName("QueryFragment")
	if !assert.NotNil(t, defn, "Could not find definition for query fragment") {
		return
	}
	// make sure that the definition has 2 selections: a field and a fragment spread
	if !assert.Len(t, defn.SelectionSet, 2, "QueryFragment in the second step had the wrong definition") {
		fmt.Println(log.FormatSelectionSet(defn.SelectionSet))
		return
	}

	var secondQFField *ast.Field
	var secondQFUserInfo *ast.FragmentSpread

	for _, selection := range defn.SelectionSet {
		if field, ok := selection.(*ast.Field); ok && field.Name == "lastName" {
			secondQFField = field
		} else if fragment, ok := selection.(*ast.FragmentSpread); ok && fragment.Name == "UserInfo" {
			secondQFUserInfo = fragment
		}
	}

	// make sure the field exists
	if !assert.NotNil(t, secondQFField,
		"could not find field under QueryFragment") {
		return
	}

	// make sure that the fragment exists
	if !assert.NotNil(t, secondQFUserInfo,
		"could not find fragment spread under QueryFragment") {
		return
	}

	userInfoDefn := secondStep.FragmentDefinitions.ForName("UserInfo")
	if !assert.NotNil(t, userInfoDefn, "Could not find definition for user info in second step.") {
		return
	}
	// there should be 1 selection under it (lastName)
	if !assert.Len(t, userInfoDefn.SelectionSet, 1, "UserInfo fragment had the wrong number of selections. Expected 1 encountered %v", len(userInfoDefn.SelectionSet)) {
		return
	}
	userInfoSelection, ok := userInfoDefn.SelectionSet[0].(*ast.Field)
	if !assert.True(t, ok, "user info selection was not a field") {
		return
	}

	assert.Equal(t, "lastName", userInfoSelection.Name)

}

func TestPlannerBuildQuery_query(t *testing.T) {
	// if we pass a query on Query to the builder we should get that same
	// selection set present in the operation without any nesting
	selection := ast.SelectionSet{
		&ast.Field{
			Name: "allUsers",
			Definition: &ast.FieldDefinition{
				Type: ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
			},
			SelectionSet: ast.SelectionSet{
				&ast.Field{
					Name: "firstName",
				},
			},
		},
	}

	variables := ast.VariableDefinitionList{
		{
			Variable: "Foo",
			Type:     ast.NamedType("String", &ast.Position{}),
		},
	}

	// the query we're building goes to the top level Query object
	operation := plannerBuildQuery("hoopla", "Query", variables, selection, ast.FragmentDefinitionList{})
	if operation == nil {
		t.Error("Did not receive a query.")
		return
	}

	// it should be a query
	assert.Equal(t, ast.Query, operation.Operations[0].Operation)
	assert.Equal(t, variables, operation.Operations[0].VariableDefinitions)
	assert.Equal(t, "hoopla", operation.Operations[0].Name)

	// the selection set should be the same as what we passed in
	assert.Equal(t, selection, operation.Operations[0].SelectionSet)
}

func TestPlannerBuildQuery_node(t *testing.T) {
	// if we are querying a specific type/id then we need to perform a query similar to
	// {
	// 		node(id: $id) {
	// 			... on User {
	// 				firstName
	// 			}
	// 		}
	// }

	// the type we are querying
	objType := "User"

	// we only need the first name for this query
	selection := ast.SelectionSet{
		&ast.Field{
			Name: "firstName",
			Definition: &ast.FieldDefinition{
				Type: ast.NamedType("String", &ast.Position{}),
			},
		},
	}

	// the query we're building goes to the User object
	operation := plannerBuildQuery("", objType, ast.VariableDefinitionList{}, selection, ast.FragmentDefinitionList{})
	if operation == nil {
		t.Error("Did not receive a query.")
		return
	}

	// it should be a query
	assert.Equal(t, ast.Query, operation.Operations[0].Operation)

	// operation name should be blank
	assert.Equal(t, "", operation.Operations[0].Name)

	// there should be one selection (node) with an argument for the id
	if len(operation.Operations[0].SelectionSet) != 1 {
		t.Error("Did not find the right number of fields on the top query")
		return
	}

	// grab the node field
	node, ok := operation.Operations[0].SelectionSet[0].(*ast.Field)
	if !ok {
		t.Error("root is not a field")
		return
	}
	if node.Name != "node" {
		t.Error("Did not ask for node at the top")
		return
	}
	// there should be one argument (id)
	if len(node.Arguments) != 1 {
		t.Error("Found the wrong number of arguments for the node field")
		return
	}
	argument := node.Arguments[0]
	if argument.Name != "id" {
		t.Error("Did not pass id to the node field")
		return
	}
	if argument.Value.Raw != "id" {
		t.Error("Did not pass the right id value to the node field")
		return
	}
	if argument.Value.Kind != ast.Variable {
		t.Error("Argument was incorrect type")
		return
	}

	// make sure the field has an inline fragment for the type
	if len(node.SelectionSet) != 1 {
		t.Error("Did not have any sub selection of the node field")
		return
	}
	fragment, ok := node.SelectionSet[0].(*ast.InlineFragment)
	if !ok {
		t.Error("Could not find inline fragment under node")
		return
	}

	// make sure its for the right type
	if fragment.TypeCondition != objType {
		t.Error("Inline fragment was for wrong type")
		return
	}

	// make sure the selection set is what we expected
	assert.Equal(t, selection, fragment.SelectionSet)
}

func TestPlanQuery_mutationsInSeries(t *testing.T) {
	t.Skip("Not implemented")
}

func TestPlanQuery_forcedPriorityResolution(t *testing.T) {
	location1 := "url1"
	location2 := "url2"

	type testCase struct {
		priorities       []string
		allUsersLocation string
		lastNameLocation string
	}

	// The location map for fields for this query.
	// All fields live on location1. "lastName" is
	// additionally available on location2.
	locations := FieldURLMap{}
	locations.RegisterURL("Query", "allUsers", location1)
	locations.RegisterURL("User", "firstName", location1)
	locations.RegisterURL("User", "lastName", location1)
	locations.RegisterURL("User", "lastName", location2)

	schema, _ := graphql.LoadSchema(`
		type User {
			firstName: String!
			lastName: String!
		}

		type Query {
			allUsers: [User!]!
		}
	`)

	// plan function creates a plan based on the passed in priorities
	plan := func(priorities []string) (QueryPlanList, error) {
		planner := (&MinQueriesPlanner{}).WithLocationPriorities(priorities)

		selections, err := planner.Plan(&PlanningContext{
			Query: `
				{
					allUsers {
						firstName
						lastName
					}
				}
			`,
			Schema:    schema,
			Locations: locations,
		})

		if err != nil {
			return nil, fmt.Errorf("encountered error when planning query: %s", err.Error())
		}

		return selections, nil
	}

	// Test case 1:
	//
	// Plan with no manually defined priorities.
	// locality rules dictate that "lastName" should
	// be resolved at location1, since it is avaiable
	// in both locations but the parent "allUsers"
	// query only lives on location1.

	selections, err := plan([]string{})
	if err != nil {
		t.Errorf("test setup failed: %s", err)
		return
	}

	// There is only one root-level query (allUsers), so
	// there should be only one step off the root
	assert.Equal(t, 1, len(selections[0].RootStep.Then))
	allUsersStep := selections[0].RootStep.Then[0]

	assert.Equal(t, location1, allUsersStep.Queryer.(*graphql.SingleRequestQueryer).URL())

	// All fields under allUsers can be resolved at the same
	// location in this case, so there should be no next step.
	allUsersField := graphql.SelectedFields(allUsersStep.SelectionSet)[0]
	assert.Equal(t, "allUsers", allUsersField.Name)
	assert.Equal(t, 0, len(allUsersStep.Then))

	// Test case 2:
	//
	// Plan with manually defined priorities.
	// location2 is prioritized over location1,
	// so the planner should ignore locality and
	// resolve lastName at location 2.

	selections, err = plan([]string{location2})
	if err != nil {
		t.Errorf("test setup failed: %s", err)
		return
	}

	// There is only one root-level query (allUsers), so
	// there should be only one step off the root
	assert.Equal(t, 1, len(selections[0].RootStep.Then))
	allUsersStep = selections[0].RootStep.Then[0]

	assert.Equal(t, location1, allUsersStep.Queryer.(*graphql.SingleRequestQueryer).URL())

	allUsersField = graphql.SelectedFields(allUsersStep.SelectionSet)[0]
	assert.Equal(t, "allUsers", allUsersField.Name)

	// lastName will be resolved on location2, due to the
	// priorities list, so there should be another step
	// to retrieve that field from the other location.
	assert.Equal(t, 1, len(allUsersStep.Then))
	lastNameStep := allUsersStep.Then[0]

	// We should only be requesting "lastName" from the other location
	lastNameSelections := graphql.SelectedFields(lastNameStep.SelectionSet)
	assert.Equal(t, 1, len(lastNameSelections))

	lastNameField := lastNameSelections[0]
	assert.Equal(t, "lastName", lastNameField.Name)
	assert.Equal(t, location2, lastNameStep.Queryer.(*graphql.SingleRequestQueryer).URL())
}
