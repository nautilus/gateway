package gateway

import (
	"fmt"
	"testing"

	"github.com/alecaivazis/graphql-gateway/graphql"
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
	plans, err := (&MinQueriesPlanner{}).Plan(`
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
	root := plans[0].RootStep.Then[0]
	// there should only be one selection
	if len(root.SelectionSet) != 1 {
		t.Error("encountered the wrong number of selections under root step")
		return
	}
	rootField := graphql.SelectedFields(root.SelectionSet)[0]

	// make sure that the first step is pointed at the right place
	queryer := root.Queryer.(*graphql.NetworkQueryer)
	assert.Equal(t, location, queryer.URL)

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
	plans, err := (&MinQueriesPlanner{}).Plan(`
		query MyQuery {
			...Foo
		}

		fragment Foo on Query {
			foo
		}
	`, schema, locations)
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when planning query: %s", err.Error())
		return
	}

	if len(plans[0].RootStep.Then) == 0 {
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
		t.Error("Root selection was not a fragment spread")
		return
	}

	// make sure that the fragment has the right name
	assert.Equal(t, "Foo", fragment.Name)

	// we need to make sure that the fragment definition matches expectation
	fragmentDef := root.FragmentDefinitions.ForName("Foo")
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
	plans, err := (&MinQueriesPlanner{}).Plan(`
		query MyQuery {
			...Foo
		}

		fragment Foo on Query {
			foo
			bar
		}
	`, schema, locations)
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
	plans, err := (&MinQueriesPlanner{}).Plan(`
		query MyQuery {
			... on Query {
				foo
				bar
			}
		}
	`, schema, locations)
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

	plans, err := (&MinQueriesPlanner{}).Plan(`
		query MyQuery {
			... on Query {
				... on Query {
					foo
				}
				bar
			}
		}
	`, schema, locations)
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
		if queryer, ok := step.Queryer.(*graphql.NetworkQueryer); ok {
			if queryer.URL == loc1 {
				loc1Step = step
			} else if queryer.URL == loc2 {
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
	selections, err := (&MinQueriesPlanner{}).Plan(`
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
	`, schema, locations)
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
	queryer := rootStep.Queryer.(*graphql.NetworkQueryer)
	assert.Equal(t, location, queryer.URL)

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

	plans, err := (&MinQueriesPlanner{}).Plan(`
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
	`, schema, locations)
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
	queryer := firstStep.Queryer.(*graphql.NetworkQueryer)
	assert.Equal(t, userLocation, queryer.URL)

	// make sure it is for allUsers
	assert.Equal(t, "allUsers", firstField.Name)

	// all users should have only one selected value since `catPhotos` is from another service
	// there will also be an `id` added so that the query can be stitched together
	if len(firstField.SelectionSet) > 2 {
		for _, selection := range graphql.SelectedFields(firstField.SelectionSet) {
			fmt.Println(selection.Name)
		}
		t.Error("Encountered too many fields on allUsers selection set")
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
	queryer = secondStep.Queryer.(*graphql.NetworkQueryer)
	assert.Equal(t, catLocation, queryer.URL)
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
	queryer = thirdStep.Queryer.(*graphql.NetworkQueryer)
	assert.Equal(t, userLocation, queryer.URL)
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

	plans, err := (&MinQueriesPlanner{}).Plan(`
		{
			allUsers {
				id
			}
		}
	`, schema, locations)
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

	plans, err := (&MinQueriesPlanner{}).Plan(`
		{
			allUsers {
				favoriteCatSpecies
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
	plans, err := (&MinQueriesPlanner{}).Plan(`
		query($id: ID!, $category: String!) {
			user(id: $id) {
				favoriteCatPhoto(category: $category, owner:$id) {
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

func TestPreparePlanQueries(t *testing.T) {
	// if we have a plan that depends on another, we need to add the id field to the selection set if
	// its not there

	thirdLevelChild := &QueryPlanStep{
		InsertionPoint: []string{"followers", "users", "friends", "followers"},
		SelectionSet: ast.SelectionSet{
			&ast.Field{
				Name:  "firstName",
				Alias: "firstName",
				Definition: &ast.FieldDefinition{
					Type: ast.NamedType("String", &ast.Position{}),
				},
			},
		},
	}

	childStep := &QueryPlanStep{
		InsertionPoint: []string{"followers", "users", "friends"},
		SelectionSet: ast.SelectionSet{
			&ast.Field{
				Name:  "followers",
				Alias: "followers",
				Definition: &ast.FieldDefinition{
					Type: ast.NonNullListType(ast.NamedType("String", &ast.Position{}), &ast.Position{}),
				},
				SelectionSet: ast.SelectionSet{},
			},
		},
		Then: []*QueryPlanStep{thirdLevelChild},
	}

	parentStep := &QueryPlanStep{
		InsertionPoint: []string{"followers"},
		SelectionSet: ast.SelectionSet{
			&ast.Field{
				Name:  "users",
				Alias: "users",
				Definition: &ast.FieldDefinition{
					Type: ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
				},
				SelectionSet: ast.SelectionSet{
					&ast.Field{
						Name:  "friends",
						Alias: "friends",
						Definition: &ast.FieldDefinition{
							Type: ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
						},
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Name:  "lastName",
								Alias: "lastName",
								Definition: &ast.FieldDefinition{
									Type: ast.NamedType("String", &ast.Position{}),
								},
							},
						},
					},
				},
			},
		},
		Then: []*QueryPlanStep{childStep},
	}

	plan := &QueryPlan{
		Operation: &ast.OperationDefinition{
			VariableDefinitions: ast.VariableDefinitionList{},
		},
		RootStep: parentStep,
	}

	// add the id fields
	err := (&MinQueriesPlanner{}).preparePlanQueries(plan, plan.RootStep)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure we assigned a query document and string to the parent step
	if parentStep.QueryDocument == nil {
		t.Error("Encountered a nil query document on parent")
	}
	if parentStep.QueryString == "" {
		t.Error("Encountered an empty query string on parent")
	}

	// we should have added `id` to the
	usersSelection, ok := parentStep.SelectionSet[0].(*ast.Field)
	if !ok {
		t.Error("users field was not a field")
		return
	}
	friendsSelection, ok := usersSelection.SelectionSet[0].(*ast.Field)
	if !ok {
		t.Error("friends field was not a field")
		return
	}

	// we should have 2 field
	if len(friendsSelection.SelectionSet) != 2 {
		t.Errorf("Encountered incorrect number of selections under friends field: Expected 2, found %v", len(friendsSelection.SelectionSet))
		return
	}

	// those 2 fields should be lastName and id
	for _, field := range graphql.SelectedFields(friendsSelection.SelectionSet) {
		if field.Name != "lastName" && field.Name != "id" {
			t.Errorf("Encountered unknown field: %v", field.Name)
		}
	}

	// make sure we assigned a query document and string to the child step
	if childStep.QueryDocument == nil {
		t.Error("Encountered a nil query document on parent")
	}
	if childStep.QueryString == "" {
		t.Error("Encountered an empty query string on parent")
	}

	// make sure the followers selection of the child has an id in it
	if len(graphql.SelectedFields(childStep.SelectionSet)[0].SelectionSet) != 1 {
		t.Errorf("Encountered incorrect number of fields under secondStep.followers: %v", len(graphql.SelectedFields(childStep.SelectionSet)[0].SelectionSet))
	}

	assert.Equal(t, "id", graphql.SelectedFields(graphql.SelectedFields(childStep.SelectionSet)[0].SelectionSet)[0].Name)
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
	operation := plannerBuildQuery("Query", variables, selection, ast.FragmentDefinitionList{})
	if operation == nil {
		t.Error("Did not receive a query.")
		return
	}

	// it should be a query
	assert.Equal(t, ast.Query, operation.Operations[0].Operation)
	assert.Equal(t, variables, operation.Operations[0].VariableDefinitions)

	// the selection set should be the same as what we passed in
	assert.Equal(t, selection, operation.Operations[0].SelectionSet)
}

func TestPlannerBuildQuery_node(t *testing.T) {
	// if we are querying a specific type/id then we need to perform a query similar to
	// {
	// 		node(id: "1234") {
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
	operation := plannerBuildQuery(objType, ast.VariableDefinitionList{}, selection, ast.FragmentDefinitionList{})
	if operation == nil {
		t.Error("Did not receive a query.")
		return
	}

	// it should be a query
	assert.Equal(t, ast.Query, operation.Operations[0].Operation)

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

func TestPlannerBuildQuery_addIDsToFragments(t *testing.T) {
	t.Error("YOU SHALL NOT PASS")
}

func TestPlanQuery_mutationsInSeries(t *testing.T) {
	t.Skip("Not implemented")
}
