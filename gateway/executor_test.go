package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

func TestExecutor_plansOfOne(t *testing.T) {
	t.Skip()
	// build a query plan that the executor will follow
	result, err := (&ParallelExecutor{}).Execute(&QueryPlan{
		RootStep: &QueryPlanStep{
			// this is equivalent to
			// query { values }
			ParentType: "Query",
			SelectionSet: ast.SelectionSet{
				&ast.Field{
					Name: "values",
					Definition: &ast.FieldDefinition{
						Type: ast.ListType(ast.NamedType("String", &ast.Position{}), &ast.Position{}),
					},
				},
			},
			// return a known value we can test against
			Queryer: &MockQueryer{JSONObject{
				"values": []string{
					"hello",
					"world",
				},
			}},
		},
	})
	if err != nil {
		t.Errorf("Encountered error executing plan: %v", err.Error())
	}

	// get the result back
	values, ok := result["values"]
	if !ok {
		t.Errorf("Did not get any values back from the execution")
	}

	// make sure we got the right values back
	assert.Equal(t, []string{"hello", "world"}, values)
}

func TestExecutor_plansWithDependencies(t *testing.T) {
	t.Skip()
	// the query we want to execute is
	// {
	// 		user {                   <- from serviceA
	//      	firstName            <- from serviceA
	// 			favoriteCatPhoto {   <- from serviceB
	// 				url              <- from serviceB
	// 			}
	// 		}
	// }

	// build a query plan that the executor will follow
	result, err := (&ParallelExecutor{}).Execute(&QueryPlan{
		RootStep: &QueryPlanStep{
			// this is equivalent to
			// query { user }
			ParentType:     "Query",
			InsertionPoint: []string{},
			SelectionSet: ast.SelectionSet{
				&ast.Field{
					Name: "user",
					Definition: &ast.FieldDefinition{
						Type: ast.NamedType("User", &ast.Position{}),
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
			// return a known value we can test against
			Queryer: &MockQueryer{JSONObject{
				"user": JSONObject{
					"firstName": "hello",
				},
			}},
			// then we have to ask for the users favorite cat photo and its url
			Then: []*QueryPlanStep{
				{
					ParentType:     "User",
					InsertionPoint: []string{"user", "favoriteCatPhoto"},
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "favoriteCatPhoto",
							Definition: &ast.FieldDefinition{
								Type: ast.NamedType("User", &ast.Position{}),
							},
							SelectionSet: ast.SelectionSet{
								&ast.Field{
									Name: "url",
									Definition: &ast.FieldDefinition{
										Type: ast.NamedType("String", &ast.Position{}),
									},
								},
							},
						},
					},
					Queryer: &MockQueryer{JSONObject{
						"node": JSONObject{
							"favoriteCatPhoto": JSONObject{
								"url": "hello world",
							},
						},
					}},
				},
			},
		},
	})
	if err != nil {
		t.Errorf("Encountered error executing plan: %v", err.Error())
		return
	}

	// make sure we got the right values back
	assert.Equal(t, JSONObject{
		"user": JSONObject{
			"firstName": "hello",
			"favoriteCatPhoto": JSONObject{
				"url": "hello world",
			},
		},
	}, result)
}

func TestExecutor_insertIntoLists(t *testing.T) {
	t.Skip()
	// the query we want to execute is
	// {
	// 		users {                  	<- Query.services @ serviceA
	//      	firstName
	//          friends {
	//              firstName
	//              photoGallery {   	<- User.photoGallery @ serviceB
	// 			    	url
	// 					followers {
	//                  	firstName	<- User.firstName @ serviceA
	//                  }
	// 			    }
	//          }
	// 		}
	// }

	// values to test against
	favoritePhotoURL := "favorite-photo-url"
	photoGalleryURL := "photoGalleryURL"
	followerName := "John"

	// build a query plan that the executor will follow
	result, err := (&ParallelExecutor{}).Execute(&QueryPlan{
		// the first step is to get Query.users
		RootStep: &QueryPlanStep{
			ParentType:     "Query",
			InsertionPoint: []string{},

			SelectionSet: ast.SelectionSet{
				&ast.Field{
					Name: "users",
					Definition: &ast.FieldDefinition{
						Type: ast.NamedType("User", &ast.Position{}),
					},
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "firstName",
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
									Definition: &ast.FieldDefinition{
										Type: ast.NamedType("String", &ast.Position{}),
									},
									Name: "firstName",
								},
							},
						},
					},
				},
			},
			// planner will actually leave behind a queryer that hits service A
			// for testing we can just return a known value
			Queryer: &MockQueryer{JSONObject{
				"users": []JSONObject{
					{
						"firstName": "hello",
						"friends": []JSONObject{
							{
								"firstName": "John",
								"id":        "1",
							},
							{
								"firstName": "Jacob",
								"id":        "2",
							},
						},
					},
					{
						"firstName": "goodbye",
						"friends": []JSONObject{
							{
								"firstName": "Jingleheymer",
								"id":        "1",
							},
							{
								"firstName": "Schmidt",
								"id":        "2",
							},
						},
					},
				},
			}},
			// then we have to ask for the users photo gallery
			Then: []*QueryPlanStep{
				// a query to satisfy User.photoGallery
				{
					ParentType:     "User",
					InsertionPoint: []string{"users", "friends", "photoGallery"},
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "photoGallery",
							Definition: &ast.FieldDefinition{
								Type: ast.ListType(ast.NamedType("CatPhoto", &ast.Position{}), &ast.Position{}),
							},
							SelectionSet: ast.SelectionSet{
								&ast.Field{
									Name: "url",
									Definition: &ast.FieldDefinition{
										Type: ast.NamedType("String", &ast.Position{}),
									},
								},
							},
						},
					},
					// planner will actually leave behind a queryer that hits service B
					// for testing we can just return a known value
					Queryer: &MockQueryer{JSONObject{
						"node": JSONObject{
							"photoGallery": JSONObject{
								"url": photoGalleryURL,
							},
						},
					}},
					Then: []*QueryPlanStep{
						// a query to satisfy User.firstName
						{
							ParentType:     "User",
							InsertionPoint: []string{"users", "friends", "photoGallery", "followers", "firstName"},
							SelectionSet: ast.SelectionSet{
								&ast.Field{
									Name: "firstName",
									Definition: &ast.FieldDefinition{
										Type: ast.NamedType("String", &ast.Position{}),
									},
								},
							},
							// planner will actually leave behind a queryer that hits service B
							// for testing we can just return a known value
							Queryer: &MockQueryer{JSONObject{
								"node": JSONObject{
									"firstName": followerName,
								},
							}},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Errorf("Encountered error executing plan: %v", err.Error())
		return
	}

	// atm the mock queryer always returns the same value so we will end up with
	// the same User.favoritePhoto and User.photoGallery
	assert.Equal(t, JSONObject{
		"users": []JSONObject{
			{
				"firstName": "hello",
				"friends": []JSONObject{
					{
						"firstName": "John",
						"photoGallery": []JSONObject{
							{
								"url": photoGalleryURL,
								"followers": []JSONObject{
									{
										"firstName": followerName,
									},
								},
							},
						},
					},
					{
						"firstName": "Jacob",
						"photoGallery": []JSONObject{
							{
								"url": photoGalleryURL,
								"followers": []JSONObject{
									{
										"firstName": followerName,
									},
								},
							},
						},
					},
				},
				"favoritePhoto": JSONObject{
					"url": favoritePhotoURL,
				},
			},
			{
				"firstName": "goodbye",
				"friends": []JSONObject{
					{
						"firstName": "Jingleheymer",
						"photoGallery": []JSONObject{
							{
								"url": photoGalleryURL,
								"followers": []JSONObject{
									{
										"firstName": followerName,
									},
								},
							},
						},
					},
					{
						"firstName": "Schmidt",
						"photoGallery": []JSONObject{
							{
								"url": photoGalleryURL,
								"followers": []JSONObject{
									{
										"firstName": followerName,
									},
								},
							},
						},
					},
				},
			},
		},
	}, result)
}

func TestFindInsertionPoint_rootList(t *testing.T) {
	// in this example, the step before would have just resolved (need to be inserted at)
	// ["users", "photoGallery"]. There would be an id field underneath each photo in the list
	// of users.photoGallery

	// we want the list of insertion points that point to
	planInsertionPoint := []string{"users", "photoGallery", "likedBy", "firstName"}

	// pretend we are in the middle of stitching a larger object
	startingPoint := [][]string{{"users:0"}}

	// there are 6 total insertion points in this example
	finalInsertionPoint := [][]string{
		// photo 0 is liked by 2 users whose firstName we have to resolve
		{"users:0", "photoGallery:0", "likedBy:0#1", "firstName"},
		{"users:0", "photoGallery:0", "likedBy:1#2", "firstName"},
		// photo 1 is liked by 3 users whose firstName we have to resolve
		{"users:0", "photoGallery:1", "likedBy:0#3", "firstName"},
		{"users:0", "photoGallery:1", "likedBy:1#4", "firstName"},
		{"users:0", "photoGallery:1", "likedBy:2#5", "firstName"},
		// photo 2 is liked by 1 user whose firstName we have to resolve
		{"users:0", "photoGallery:2", "likedBy:0#6", "firstName"},
	}

	// the selection we're going to make
	stepSelectionSet := ast.SelectionSet{
		&ast.Field{
			Name: "photoGallery",
			Definition: &ast.FieldDefinition{
				Type: ast.ListType(ast.NamedType("Photo", &ast.Position{}), &ast.Position{}),
			},
			SelectionSet: ast.SelectionSet{
				&ast.Field{
					Name: "likedBy",
					Definition: &ast.FieldDefinition{
						Type: ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
					},
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "totalLikes",
							Definition: &ast.FieldDefinition{
								Type: ast.NamedType("Int", &ast.Position{}),
							},
						},
						&ast.Field{
							Name: "id",
							Definition: &ast.FieldDefinition{
								Type: ast.NamedType("ID", &ast.Position{}),
							},
						},
					},
				},
			},
		},
	}

	// the result of the step
	result := JSONObject{
		"photoGallery": []JSONObject{
			{
				"likedBy": []JSONObject{
					{
						"totalLikes": 10,
						"id":         "1",
					},
					{
						"totalLikes": 10,
						"id":         "2",
					},
				},
			},
			{
				"likedBy": []JSONObject{
					{
						"totalLikes": 10,
						"id":         "3",
					},
					{
						"totalLikes": 10,
						"id":         "4",
					},
					{
						"totalLikes": 10,
						"id":         "5",
					},
				},
			},
			{
				"likedBy": []JSONObject{
					{
						"totalLikes": 10,
						"id":         "6",
					},
				},
			},
		},
	}

	generatedPoint, err := findInsertionPoints(planInsertionPoint, stepSelectionSet, result, startingPoint)
	if err != nil {
		t.Error(t, err)
		return
	}

	assert.Equal(t, finalInsertionPoint, generatedPoint)
}

func TestFindInsertionPoint_stitchIntoObject(t *testing.T) {
	// we want the list of insertion points that point to
	planInsertionPoint := []string{"users", "photoGallery", "author", "firstName"}

	// pretend we are in the middle of stitching a larger object
	startingPoint := [][]string{{"users:0"}}

	// there are 6 total insertion points in this example
	finalInsertionPoint := [][]string{
		// photo 0 is liked by 2 users whose firstName we have to resolve
		{"users:0", "photoGallery:0", "author#1", "firstName"},
		// photo 1 is liked by 3 users whose firstName we have to resolve
		{"users:0", "photoGallery:1", "author#2", "firstName"},
		// photo 2 is liked by 1 user whose firstName we have to resolve
		{"users:0", "photoGallery:2", "author#3", "firstName"},
	}

	// the selection we're going to make
	stepSelectionSet := ast.SelectionSet{
		&ast.Field{
			Name: "photoGallery",
			Definition: &ast.FieldDefinition{
				Type: ast.ListType(ast.NamedType("Photo", &ast.Position{}), &ast.Position{}),
			},
			SelectionSet: ast.SelectionSet{
				&ast.Field{
					Name: "author",
					Definition: &ast.FieldDefinition{
						Type: ast.NamedType("User", &ast.Position{}),
					},
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "totalLikes",
							Definition: &ast.FieldDefinition{
								Type: ast.NamedType("Int", &ast.Position{}),
							},
						},
						&ast.Field{
							Name: "id",
							Definition: &ast.FieldDefinition{
								Type: ast.NamedType("ID", &ast.Position{}),
							},
						},
					},
				},
			},
		},
	}

	// the result of the step
	result := JSONObject{
		"photoGallery": []JSONObject{
			{
				"author": JSONObject{
					"id": "1",
				},
			},
			{
				"author": JSONObject{
					"id": "2",
				},
			},
			{
				"author": JSONObject{
					"id": "3",
				},
			},
		},
	}

	generatedPoint, err := findInsertionPoints(planInsertionPoint, stepSelectionSet, result, startingPoint)
	if err != nil {
		t.Error(t, err)
		return
	}

	assert.Equal(t, finalInsertionPoint, generatedPoint)

}
