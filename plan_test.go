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
	`, schema, locations, ast.VariableDefinitionList{})
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
	rootField := selectedFields(root.SelectionSet)[0]

	// make sure that the first step is pointed at the right place
	queryer := root.Queryer.(*graphql.NetworkQueryer)
	assert.Equal(t, location, queryer.URL)

	// we need to be asking for Query.foo
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
	`, schema, locations, ast.VariableDefinitionList{})
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

	rootField := selectedFields(rootStep.SelectionSet)[0]

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
	`, schema, locations, ast.VariableDefinitionList{})
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
		t.Error("first strep did not have a selection set")
		return
	}
	firstField := selectedFields(firstStep.SelectionSet)[0]
	// it is resolved against the user service
	queryer := firstStep.Queryer.(*graphql.NetworkQueryer)
	assert.Equal(t, userLocation, queryer.URL)

	// make sure it is for allUsers
	assert.Equal(t, "allUsers", firstField.Name)

	// all users should have only one selected value since `catPhotos` is from another service
	if len(firstField.SelectionSet) > 1 {
		for _, selection := range selectedFields(firstField.SelectionSet) {
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
	assert.Equal(t, []string{"allUsers", "catPhotos"}, secondStep.InsertionPoint)

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
	selectedSecondField := selectedFields(secondStep.SelectionSet)[0]
	assert.Equal(t, "catPhotos", selectedSecondField.Name)

	// we should have also asked for one field underneath
	secondSubSelection := selectedFields(selectedSecondField.SelectionSet)
	if len(secondSubSelection) != 1 {
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
	assert.Equal(t, []string{"allUsers", "catPhotos", "owner"}, thirdStep.InsertionPoint)

	// make sure we are grabbing values off of User since we asked for User.catPhotos
	assert.Equal(t, "CatPhoto", thirdStep.ParentType)
	// we should be going to the catePhoto service
	queryer = thirdStep.Queryer.(*graphql.NetworkQueryer)
	assert.Equal(t, userLocation, queryer.URL)
	// we should only want one field selected
	if len(thirdStep.SelectionSet) != 1 {
		t.Errorf("Did not have the right number of subfields of User.catPhotos: %v", len(thirdStep.SelectionSet))
		return
	}

	// make sure we selected the catPhotos field
	selectedThirdField := selectedFields(thirdStep.SelectionSet)[0]
	assert.Equal(t, "owner", selectedThirdField.Name)

	// we should have also asked for one field underneath
	thirdSubSelection := selectedFields(selectedThirdField.SelectionSet)
	if len(thirdSubSelection) != 1 {
		t.Error("Encountered the incorrect number of fields selected under User.catPhotos")
	}
	thirdSubSelectionField := thirdSubSelection[0]
	assert.Equal(t, "firstName", thirdSubSelectionField.Name)
}

func TestApplyFragments_mergesFragments(t *testing.T) {
	// a selection set representing
	// {
	//      birthday
	// 		... on User {
	// 			firstName
	//			lastName
	// 			friends {
	// 				firstName
	// 			}
	// 		}
	//      ...SecondFragment
	// 	}
	//
	// 	fragment SecondFragment on User {
	// 		lastName
	// 		friends {
	// 			lastName
	//			friends {
	//				lastName
	//			}
	// 		}
	// 	}
	//
	//
	// should be flattened into
	// {
	//		birthday
	// 		firstName
	// 		lastName
	// 		friends {
	// 			firstName
	// 			lastName
	//			friends {
	//				lastName
	//			}
	// 		}
	// }
	selectionSet := ast.SelectionSet{
		&ast.Field{
			Name: "birthday",
			Definition: &ast.FieldDefinition{
				Type: ast.NamedType("DateTime", &ast.Position{}),
			},
		},
		&ast.FragmentSpread{
			Name: "SecondFragment",
		},
		&ast.InlineFragment{
			TypeCondition: "User",
			SelectionSet: ast.SelectionSet{
				&ast.Field{
					Name: "firstName",
					Definition: &ast.FieldDefinition{
						Type: ast.NamedType("String", &ast.Position{}),
					},
				},
				&ast.Field{
					Name: "lastName",
					Definition: &ast.FieldDefinition{
						Type: ast.NamedType("String", &ast.Position{}),
					},
				},
				&ast.Field{
					Name: "friends",
					Definition: &ast.FieldDefinition{
						Type: ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
					},
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "firstName",
							Definition: &ast.FieldDefinition{
								Type: ast.NamedType("String", &ast.Position{}),
							},
						},
					},
				},
			},
		},
	}

	fragmentDefinition := ast.FragmentDefinitionList{
		&ast.FragmentDefinition{
			Name: "SecondFragment",
			SelectionSet: ast.SelectionSet{
				&ast.Field{
					Name: "lastName",
					Definition: &ast.FieldDefinition{
						Type: ast.NamedType("String", &ast.Position{}),
					},
				},
				&ast.Field{
					Name: "friends",
					Definition: &ast.FieldDefinition{
						Type: ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
					},
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "lastName",
							Definition: &ast.FieldDefinition{
								Type: ast.NamedType("String", &ast.Position{}),
							},
						},
						&ast.Field{
							Name: "friends",
							Definition: &ast.FieldDefinition{
								Type: ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
							},
							SelectionSet: ast.SelectionSet{
								&ast.Field{
									Name: "lastName",
									Definition: &ast.FieldDefinition{
										Type: ast.NamedType("String", &ast.Position{}),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// flatten the selection
	finalSelection := applyFragments(selectionSet, fragmentDefinition, ast.VariableDefinitionList{})

	// make sure there are 4 fields at the root of the selection
	if len(finalSelection) != 4 {
		t.Error("Encountered the inccorect number of selections")
		return
	}
}

func TestApplyFragments_skipAndIncludeDirectives(t *testing.T) {
	t.Skip("Not yet implemented")
}

func TestApplyFragments_leavesUnionsAndInterfaces(t *testing.T) {
	t.Skip("Not yet implemented")
}

func TestPlanQuery_multipleRootFields(t *testing.T) {
	t.Skip("Not implemented")
}

func TestPlanQuery_mutationsInSeries(t *testing.T) {
	t.Skip("Not implemented")
}

func TestPlanQuery_siblingFields(t *testing.T) {
	t.Skip("Not implemented")
}

func TestPlanQuery_duplicateFieldsOnEither(t *testing.T) {
	// make sure that if I have the same field defined on both schemas we dont create extraneous calls
	t.Skip("Not implemented")
}

func TestPlanQuery_groupsConflictingFields(t *testing.T) {
	// if I can find a field in 4 different services, look for the one I"m already going to
	t.Skip("Not implemented")
}

func TestPlanQuery_combineFragments(t *testing.T) {
	// fragments could bring in different fields from different services
	t.Skip("Not implemented")
}

func TestPlanQuery_threadVariables(t *testing.T) {
	t.Skip("Not implemented")
}
