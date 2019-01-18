package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

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
			Name:  "birthday",
			Alias: "birthday",
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
					Name:  "lastName",
					Alias: "lastName",
					Definition: &ast.FieldDefinition{
						Type: ast.NamedType("String", &ast.Position{}),
					},
				},
				&ast.Field{
					Name:  "firstName",
					Alias: "firstName",
					Definition: &ast.FieldDefinition{
						Type: ast.NamedType("String", &ast.Position{}),
					},
				},
				&ast.Field{
					Name:  "friends",
					Alias: "friends",
					Definition: &ast.FieldDefinition{
						Type: ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
					},
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name:  "firstName",
							Alias: "firstName",
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
					Name:  "lastName",
					Alias: "lastName",
					Definition: &ast.FieldDefinition{
						Type: ast.NamedType("String", &ast.Position{}),
					},
				},
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
		},
	}

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

	// flatten the selection
	finalSelection, err := ApplyFragments(selectionSet, fragmentDefinition)
	if err != nil {
		t.Error(err.Error())
		return
	}
	fields := SelectedFields(finalSelection)

	// make sure there are 4 fields at the root of the selection
	if len(fields) != 4 {
		t.Errorf("Encountered the incorrect number of selections: %v", len(fields))
		return
	}

	// get the selection set for birthday
	var birthdaySelection *ast.Field
	var firstNameSelection *ast.Field
	var lastNameSelection *ast.Field
	var friendsSelection *ast.Field

	for _, selection := range fields {
		switch selection.Alias {
		case "birthday":
			birthdaySelection = selection
		case "firstName":
			firstNameSelection = selection
		case "lastName":
			lastNameSelection = selection
		case "friends":
			friendsSelection = selection
		}
	}

	// make sure we got each definition
	assert.NotNil(t, birthdaySelection)
	assert.NotNil(t, firstNameSelection)
	assert.NotNil(t, lastNameSelection)
	assert.NotNil(t, friendsSelection)

	// make sure there are 3 selections under friends (firstName, lastName, and friends)
	if len(friendsSelection.SelectionSet) != 3 {
		t.Errorf("Encountered the wrong number of selections under .friends: len = %v)", len(friendsSelection.SelectionSet))
		for _, selection := range friendsSelection.SelectionSet {
			field, _ := selection.(*CollectedField)
			t.Errorf("    %s", field.Name)
		}
		return
	}
}

func TestExtractVariables(t *testing.T) {
	table := []struct {
		Name      string
		Arguments ast.ArgumentList
		Variables []string
	}{
		//  user(id: $id, name:$name) should extract ["id", "name"]
		{
			Name:      "Top Level arguments",
			Variables: []string{"id", "name"},
			Arguments: ast.ArgumentList{
				&ast.Argument{
					Name: "id",
					Value: &ast.Value{
						Kind: ast.Variable,
						Raw:  "id",
					},
				},
				&ast.Argument{
					Name: "name",
					Value: &ast.Value{
						Kind: ast.Variable,
						Raw:  "name",
					},
				},
			},
		},
		//  catPhotos(categories: [$a, "foo", $b]) should extract ["a", "b"]
		{
			Name:      "List nested arguments",
			Variables: []string{"a", "b"},
			Arguments: ast.ArgumentList{
				&ast.Argument{
					Name: "category",
					Value: &ast.Value{
						Kind: ast.ListValue,
						Children: ast.ChildValueList{
							&ast.ChildValue{
								Value: &ast.Value{
									Kind: ast.Variable,
									Raw:  "a",
								},
							},
							&ast.ChildValue{
								Value: &ast.Value{
									Kind: ast.StringValue,
									Raw:  "foo",
								},
							},
							&ast.ChildValue{
								Value: &ast.Value{
									Kind: ast.Variable,
									Raw:  "b",
								},
							},
						},
					},
				},
			},
		},
		//  users(favoriteMovieFilter: {category: $targetCategory, rating: $targetRating}) should extract ["targetCategory", "targetRating"]
		{
			Name:      "Object nested arguments",
			Variables: []string{"targetCategory", "targetRating"},
			Arguments: ast.ArgumentList{
				&ast.Argument{
					Name: "favoriteMovieFilter",
					Value: &ast.Value{
						Kind: ast.ObjectValue,
						Children: ast.ChildValueList{
							&ast.ChildValue{
								Name: "category",
								Value: &ast.Value{
									Kind: ast.Variable,
									Raw:  "targetCategory",
								},
							},
							&ast.ChildValue{
								Name: "rating",
								Value: &ast.Value{
									Kind: ast.Variable,
									Raw:  "targetRating",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, row := range table {
		t.Run(row.Name, func(t *testing.T) {
			assert.Equal(t, row.Variables, ExtractVariables(row.Arguments))
		})
	}
}
