package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"testing"

	"github.com/nautilus/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

type roundTripFunc func(req *http.Request) *http.Response

// RoundTrip .
func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestExecutor_plansOfOne(t *testing.T) {
	// build a query plan that the executor will follow
	result, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					{
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
						Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
							"values": []string{
								"hello",
								"world",
							},
						},
						},
					},
				},
			},
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
	result, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					{

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
						Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
							"user": map[string]interface{}{
								"id":        "1",
								"firstName": "hello",
							},
						}},
						// then we have to ask for the users favorite cat photo and its url
						Then: []*QueryPlanStep{
							{
								ParentType:     "User",
								InsertionPoint: []string{"user"},
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
								Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
									"node": map[string]interface{}{
										"favoriteCatPhoto": map[string]interface{}{
											"url": "hello world",
										},
									},
								}},
							},
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

	// make sure we got the right values back
	assert.Equal(t, map[string]interface{}{
		"user": map[string]interface{}{
			"id":        "1",
			"firstName": "hello",
			"favoriteCatPhoto": map[string]interface{}{
				"url": "hello world",
			},
		},
	}, result)
}

func TestExecutor_emptyPlansWithDependencies(t *testing.T) {
	// the query we want to execute is
	// {
	// 		user {                   <- from serviceA
	//      	firstName            <- from serviceA
	// 		}
	// }

	// build a query plan that the executor will follow
	result, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					{ // this is equivalent to
						// query { user }
						ParentType:     "Query",
						InsertionPoint: []string{},
						// return a known value we can test against
						Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{}},
						// then we have to ask for the users favorite cat photo and its url
						Then: []*QueryPlanStep{
							{
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
								Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
									"user": map[string]interface{}{
										"id":        "1",
										"firstName": "hello",
									},
								}},
							},
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

	// make sure we got the right values back
	assert.Equal(t, map[string]interface{}{
		"user": map[string]interface{}{
			"id":        "1",
			"firstName": "hello",
		},
	}, result)
}

func TestExecutor_insertIntoFragmentSpread(t *testing.T) {
	// the query we want to execute is
	// {
	//   photo {								<- Query.services @ serviceA
	//     ... photoFragment
	//     }
	//   }
	// }
	// fragment createdByFragment on Photo {
	//   createdBy {								<- User boundary type
	// 	 	firstName								<- User.firstName @ serviceA
	//   	address 							<- User.address @ serviceB
	//   }
	// }
	result, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					// a query to satisfy photo.createdBy.firstName
					{
						ParentType:     "Query",
						InsertionPoint: []string{},
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Alias: "photo",
								Name:  "photo",
								Definition: &ast.FieldDefinition{
									Type: ast.NamedType("Photo", &ast.Position{}),
								},
								SelectionSet: ast.SelectionSet{
									&ast.FragmentSpread{
										Name:       "photoFragment",
										Definition: nil,
									},
								},
							},
						},
						Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
							"photo": map[string]interface{}{
								"createdBy": map[string]interface{}{
									"firstName": "John",
									"id":        "1",
								},
							},
						}},
						FragmentDefinitions: ast.FragmentDefinitionList{
							&ast.FragmentDefinition{
								Name:          "photoFragment",
								TypeCondition: "Photo",
								SelectionSet: ast.SelectionSet{
									&ast.Field{
										Name: "createdBy",
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
							},
						},
						Then: []*QueryPlanStep{
							// a query to satisfy User.address
							{
								ParentType:     "User",
								InsertionPoint: []string{"photo", "createdBy"}, // photo is the query name here
								SelectionSet: ast.SelectionSet{
									&ast.FragmentSpread{
										Name: "photoFragment",
									},
								},
								Queryer: graphql.QueryerFunc(
									func(input *graphql.QueryInput) (interface{}, error) {
										assert.Equal(t, map[string]interface{}{"id": "1"}, input.Variables)
										// make sure that we got the right variable inputs
										return map[string]interface{}{
											"node": map[string]interface{}{
												"address": "addressValue",
											},
										}, nil
									},
								),
								FragmentDefinitions: ast.FragmentDefinitionList{
									&ast.FragmentDefinition{
										Name:          "photoFragment",
										TypeCondition: "User",
										SelectionSet: ast.SelectionSet{
											&ast.Field{
												Name:  "address",
												Alias: "address",
												Definition: &ast.FieldDefinition{
													Name: "address",
													Type: ast.NamedType("String", &ast.Position{}),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Errorf("Encountered error execluting plan: %v", err.Error())
		return
	}
	assert.Equal(t, map[string]interface{}{
		"photo": map[string]interface{}{
			"createdBy": map[string]interface{}{
				"id":        "1",
				"firstName": "John",
				"address":   "addressValue",
			},
		},
	}, result)
}

func TestExecutor_insertIntoListFragmentSpreads(t *testing.T) {
	// {
	// 	photos {								<-- Query.services @ serviceA, list
	// 	  ...photosFragment
	// 	}
	//   }
	//   fragment photosFragment on Photo {
	// 	createdBy {
	// 	  firstName								<-- User.firstName @ serviceA
	// 	  address								<-- User.address @ serviceB
	// 	  }
	//   }
	result, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					// a query to satisfy photo.createdBy.firstName
					{
						ParentType:     "Query",
						InsertionPoint: []string{},
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Alias: "photos",
								Name:  "photos",
								Definition: &ast.FieldDefinition{
									Type: ast.ListType(ast.NamedType("Photo", &ast.Position{}), &ast.Position{}),
								},
								SelectionSet: ast.SelectionSet{
									&ast.FragmentSpread{
										Name:       "photosFragment",
										Definition: nil,
									},
								},
							},
						},
						Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
							"photos": []interface{}{
								map[string]interface{}{
									"createdBy": map[string]interface{}{
										"firstName": "John",
										"id":        "1",
									},
								},
								map[string]interface{}{
									"createdBy": map[string]interface{}{
										"firstName": "Jane",
										"id":        "2",
									},
								},
							},
						}},
						FragmentDefinitions: ast.FragmentDefinitionList{
							&ast.FragmentDefinition{
								Name:          "photosFragment",
								TypeCondition: "Photo",
								SelectionSet: ast.SelectionSet{
									&ast.Field{
										Name: "createdBy",
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
							},
						},
						Then: []*QueryPlanStep{
							// a query to satisfy User.address
							{
								ParentType:     "User",
								InsertionPoint: []string{"photos", "createdBy"}, // photo is the query name here
								SelectionSet: ast.SelectionSet{
									&ast.FragmentSpread{
										Name: "photosFragment",
									},
								},
								Queryer: graphql.QueryerFunc(
									func(input *graphql.QueryInput) (interface{}, error) {
										assert.Contains(t, []interface{}{"1", "2"}, input.Variables["id"])
										return map[string]interface{}{
											"node": map[string]interface{}{
												"address": fmt.Sprintf("address-%s", input.Variables["id"]),
											},
										}, nil
									},
								),
								FragmentDefinitions: ast.FragmentDefinitionList{
									&ast.FragmentDefinition{
										Name:          "photosFragment",
										TypeCondition: "User",
										SelectionSet: ast.SelectionSet{
											&ast.Field{
												Name:  "address",
												Alias: "address",
												Definition: &ast.FieldDefinition{
													Name: "address",
													Type: ast.NamedType("String", &ast.Position{}),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Errorf("Encountered error execluting plan: %v", err.Error())
		return
	}
	assert.Equal(t, map[string]interface{}{
		"photos": []interface{}{
			map[string]interface{}{
				"createdBy": map[string]interface{}{
					"id":        "1",
					"firstName": "John",
					"address":   "address-1",
				},
			},
			map[string]interface{}{
				"createdBy": map[string]interface{}{
					"id":        "2",
					"firstName": "Jane",
					"address":   "address-2",
				},
			},
		},
	}, result)
}

func TestExecutor_insertIntoFragmentSpreadLists(t *testing.T) {
	// {
	// 	photo {								<-- Query.services @ serviceA
	// 	  ...photoFragment
	// 	}
	//   }
	//   fragment photoFragment on Photo {
	// 	viewedBy {								<-- list
	// 	  firstName								<-- User.firstName @ serviceA
	// 	  address								<-- User.address @ serviceB
	// 	}
	//   }
	result, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					// a query to satisfy photo.viewedBy.firstName
					{
						ParentType:     "Query",
						InsertionPoint: []string{},
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Alias: "photo",
								Name:  "photo",
								Definition: &ast.FieldDefinition{
									Type: ast.NamedType("Photo", &ast.Position{}),
								},
								SelectionSet: ast.SelectionSet{
									&ast.FragmentSpread{
										Name:       "photoFragment",
										Definition: nil,
									},
								},
							},
						},
						Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
							"photo": map[string]interface{}{
								"viewedBy": []interface{}{
									map[string]interface{}{
										"firstName": "John",
										"id":        "1",
									},
									map[string]interface{}{
										"firstName": "Jane",
										"id":        "2",
									},
								},
							},
						}},
						FragmentDefinitions: ast.FragmentDefinitionList{
							&ast.FragmentDefinition{
								Name:          "photoFragment",
								TypeCondition: "Photo",
								SelectionSet: ast.SelectionSet{
									&ast.Field{
										Name: "viewedBy",
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
						},
						Then: []*QueryPlanStep{
							// a query to satisfy User.address
							{
								ParentType:     "User",
								InsertionPoint: []string{"photo", "viewedBy"},
								SelectionSet: ast.SelectionSet{
									&ast.FragmentSpread{
										Name: "photoFragment",
									},
								},
								Queryer: graphql.QueryerFunc(
									func(input *graphql.QueryInput) (interface{}, error) {
										assert.Contains(t, []interface{}{"1", "2"}, input.Variables["id"])
										return map[string]interface{}{
											"node": map[string]interface{}{
												"address": fmt.Sprintf("address-%s", input.Variables["id"]),
											},
										}, nil
									},
								),
								FragmentDefinitions: ast.FragmentDefinitionList{
									&ast.FragmentDefinition{
										Name:          "photoFragment",
										TypeCondition: "User",
										SelectionSet: ast.SelectionSet{
											&ast.Field{
												Name:  "address",
												Alias: "address",
												Definition: &ast.FieldDefinition{
													Name: "address",
													Type: ast.NamedType("String", &ast.Position{}),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Errorf("Encountered error execluting plan: %v", err.Error())
		return
	}
	assert.Equal(t, map[string]interface{}{
		"photo": map[string]interface{}{
			"viewedBy": []interface{}{
				map[string]interface{}{
					"firstName": "John",
					"id":        "1",
					"address":   "address-1",
				},
				map[string]interface{}{
					"firstName": "Jane",
					"id":        "2",
					"address":   "address-2",
				},
			},
		},
	}, result)
}

func TestExecutor_insertIntoInlineFragment(t *testing.T) {
	// the query we want to execute is
	// {
	//   photo {								<- Query.services @ serviceA
	//     ... on Photo {
	// 			createdBy {
	// 				firstName					<- User.firstName @ serviceA
	// 				address						<- User.address @ serviceB
	// 			}
	// 	   }
	//  }
	// }
	result, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					// a query to satisfy photo.createdBy.firstName
					{
						ParentType:     "Query",
						InsertionPoint: []string{},
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Alias: "photo",
								Name:  "photo",
								Definition: &ast.FieldDefinition{
									Type: ast.NamedType("Photo", &ast.Position{}),
								},
								SelectionSet: ast.SelectionSet{
									&ast.InlineFragment{
										TypeCondition: "Photo",
										SelectionSet: ast.SelectionSet{
											&ast.Field{
												Name: "createdBy",
												Definition: &ast.FieldDefinition{
													Name: "createdBy",
													Type: ast.NamedType("User", &ast.Position{}),
												},
												SelectionSet: ast.SelectionSet{
													&ast.Field{
														Name: "firstName",
														Definition: &ast.FieldDefinition{
															Name: "firstName",
															Type: ast.NamedType("String", &ast.Position{}),
														},
													},
												},
											},
										},
									},
								},
							},
						},
						Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
							"photo": map[string]interface{}{
								"createdBy": map[string]interface{}{
									"firstName": "John",
									"id":        "1",
								},
							},
						}},
						Then: []*QueryPlanStep{
							// a query to satisfy User.address
							{
								ParentType:     "User",
								InsertionPoint: []string{"photo", "createdBy"}, // photo is the query name here
								SelectionSet: ast.SelectionSet{
									&ast.Field{
										Name: "address",
										Definition: &ast.FieldDefinition{
											Name: "address",
											Type: ast.NamedType("String", &ast.Position{}),
										},
									},
								},
								Queryer: graphql.QueryerFunc(
									func(input *graphql.QueryInput) (interface{}, error) {
										assert.Equal(t, map[string]interface{}{"id": "1"}, input.Variables)
										// make sure that we got the right variable inputs
										return map[string]interface{}{
											"node": map[string]interface{}{
												"address": "addressValue",
											},
										}, nil
									},
								),
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Errorf("Encountered error execluting plan: %v", err.Error())
		return
	}
	assert.Equal(t, map[string]interface{}{
		"photo": map[string]interface{}{
			"createdBy": map[string]interface{}{
				"id":        "1",
				"firstName": "John",
				"address":   "addressValue",
			},
		},
	}, result)
}

func TestExecutor_insertIntoListInlineFragments(t *testing.T) {
	// {
	// 	photos {								<-- Query.services @ serviceA, list
	// 	  ... on Photo {
	// 		createdBy {
	// 	  	  firstName								<-- User.firstName @ serviceA
	// 	  	  address								<-- User.address @ serviceB
	// 	    }
	// 	   }
	//  }
	// }
	result, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					// a query to satisfy photo.createdBy.firstName
					{
						ParentType:     "Query",
						InsertionPoint: []string{},
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Alias: "photos",
								Name:  "photos",
								Definition: &ast.FieldDefinition{
									Type: ast.ListType(ast.NamedType("Photo", &ast.Position{}), &ast.Position{}),
								},
								SelectionSet: ast.SelectionSet{
									&ast.InlineFragment{
										TypeCondition: "Photo",
										SelectionSet: ast.SelectionSet{
											&ast.Field{
												Name: "createdBy",
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
									},
								},
							},
						},
						Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
							"photos": []interface{}{
								map[string]interface{}{
									"createdBy": map[string]interface{}{
										"firstName": "John",
										"id":        "1",
									},
								},
								map[string]interface{}{
									"createdBy": map[string]interface{}{
										"firstName": "Jane",
										"id":        "2",
									},
								},
							},
						}},
						Then: []*QueryPlanStep{
							// a query to satisfy User.address
							{
								ParentType:     "User",
								InsertionPoint: []string{"photos", "createdBy"}, // photo is the query name here
								SelectionSet: ast.SelectionSet{
									&ast.Field{
										Name: "address",
										Definition: &ast.FieldDefinition{
											Name: "address",
											Type: ast.NamedType("String", &ast.Position{}),
										},
									},
								},
								Queryer: graphql.QueryerFunc(
									func(input *graphql.QueryInput) (interface{}, error) {
										assert.Contains(t, []interface{}{"1", "2"}, input.Variables["id"])
										return map[string]interface{}{
											"node": map[string]interface{}{
												"address": fmt.Sprintf("address-%s", input.Variables["id"]),
											},
										}, nil
									},
								),
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Errorf("Encountered error execluting plan: %v", err.Error())
		return
	}
	assert.Equal(t, map[string]interface{}{
		"photos": []interface{}{
			map[string]interface{}{
				"createdBy": map[string]interface{}{
					"id":        "1",
					"firstName": "John",
					"address":   "address-1",
				},
			},
			map[string]interface{}{
				"createdBy": map[string]interface{}{
					"id":        "2",
					"firstName": "Jane",
					"address":   "address-2",
				},
			},
		},
	}, result)
}

func TestExecutor_insertIntoInlineFragmentsList(t *testing.T) {
	// {
	// 	photo {								<-- Query.services @ serviceA
	// 	  ... on Photo {
	//       viewedBy {						<-- list
	//          firstName					<-- User.firstName @ serviceA
	//          address						<-- User.address @ serviceB
	//       }
	//    }
	// 	}
	// }
	result, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					// a query to satisfy photo.viewedBy.firstName
					{
						ParentType:     "Query",
						InsertionPoint: []string{},
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Alias: "photo",
								Name:  "photo",
								Definition: &ast.FieldDefinition{
									Type: ast.NamedType("Photo", &ast.Position{}),
								},
								SelectionSet: ast.SelectionSet{
									&ast.InlineFragment{
										TypeCondition: "Photo",
										SelectionSet: ast.SelectionSet{
											&ast.Field{
												Name: "viewedBy",
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
								},
							},
						},
						Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
							"photo": map[string]interface{}{
								"viewedBy": []interface{}{
									map[string]interface{}{
										"firstName": "John",
										"id":        "1",
									},
									map[string]interface{}{
										"firstName": "Jane",
										"id":        "2",
									},
								},
							},
						}},
						Then: []*QueryPlanStep{
							// a query to satisfy User.address
							{
								ParentType:     "User",
								InsertionPoint: []string{"photo", "viewedBy"},
								SelectionSet: ast.SelectionSet{
									&ast.Field{
										Name: "address",
										Definition: &ast.FieldDefinition{
											Name: "address",
											Type: ast.NamedType("String", &ast.Position{}),
										},
									},
								},
								Queryer: graphql.QueryerFunc(
									func(input *graphql.QueryInput) (interface{}, error) {
										t.Log("HELLOOO")
										assert.Contains(t, []interface{}{"1", "2"}, input.Variables["id"])
										return map[string]interface{}{
											"node": map[string]interface{}{
												"address": fmt.Sprintf("address-%s", input.Variables["id"]),
											},
										}, nil
									},
								),
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Errorf("Encountered error execluting plan: %v", err.Error())
		return
	}
	assert.Equal(t, map[string]interface{}{
		"photo": map[string]interface{}{
			"viewedBy": []interface{}{
				map[string]interface{}{
					"firstName": "John",
					"id":        "1",
					"address":   "address-1",
				},
				map[string]interface{}{
					"firstName": "Jane",
					"id":        "2",
					"address":   "address-2",
				},
			},
		},
	}, result)

}

func TestExecutor_insertIntoLists(t *testing.T) {
	// the query we want to execute is
	// {
	// 		users {                  	<- Query.services @ serviceA
	//      	firstName
	//          friends {				<- list
	//              firstName
	//              photoGallery {   	<- list, User.photoGallery @ serviceB
	// 			    	url
	// 					followers { .   <- list
	//                  	firstName	<- User.firstName @ serviceA
	//                  }
	// 			    }
	//          }
	// 		}
	// }

	// values to test against
	photoGalleryURL := "photoGalleryURL"
	followerName := "John"

	// build a query plan that the executor will follow
	result, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					{
						ParentType:     "Query",
						InsertionPoint: []string{},
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Name: "users",
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
						Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
							"users": []interface{}{
								map[string]interface{}{
									"firstName": "hello",
									"friends": []interface{}{
										map[string]interface{}{
											"firstName": "John",
											"id":        "1",
										},
										map[string]interface{}{
											"firstName": "Jacob",
											"id":        "2",
										},
									},
								},
								map[string]interface{}{
									"firstName": "goodbye",
									"friends": []interface{}{
										map[string]interface{}{
											"firstName": "Jingleheymer",
											"id":        "3",
										},
										map[string]interface{}{
											"firstName": "Schmidt",
											"id":        "4",
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
								InsertionPoint: []string{"users", "friends"},
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
											&ast.Field{
												Name: "followers",
												Definition: &ast.FieldDefinition{
													Type: ast.NamedType("User", &ast.Position{}),
												},
												SelectionSet: ast.SelectionSet{},
											},
										},
									},
								},
								// planner will actually leave behind a queryer that hits service B
								// for testing we can just return a known value
								Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
									"node": map[string]interface{}{
										"photoGallery": []interface{}{
											map[string]interface{}{
												"url": photoGalleryURL,
												"followers": []interface{}{
													map[string]interface{}{
														"id": "1",
													},
												},
											},
										},
									},
								}},
								Then: []*QueryPlanStep{
									// a query to satisfy User.firstName
									{
										ParentType:     "User",
										InsertionPoint: []string{"users", "friends", "photoGallery", "followers"},
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
										Queryer: graphql.QueryerFunc(
											func(input *graphql.QueryInput) (interface{}, error) {
												// make sure that we got the right variable inputs
												assert.Equal(t, map[string]interface{}{"id": "1"}, input.Variables)

												// return the payload
												return map[string]interface{}{
													"node": map[string]interface{}{
														"firstName": followerName,
													},
												}, nil
											},
										),
									},
								},
							},
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
	assert.Equal(t, map[string]interface{}{
		"users": []interface{}{
			map[string]interface{}{
				"firstName": "hello",
				"friends": []interface{}{
					map[string]interface{}{
						"firstName": "John",
						"id":        "1",
						"photoGallery": []interface{}{
							map[string]interface{}{
								"url": photoGalleryURL,
								"followers": []interface{}{
									map[string]interface{}{
										"id":        "1",
										"firstName": followerName,
									},
								},
							},
						},
					},
					map[string]interface{}{
						"firstName": "Jacob",
						"id":        "2",
						"photoGallery": []interface{}{
							map[string]interface{}{
								"url": photoGalleryURL,
								"followers": []interface{}{
									map[string]interface{}{
										"id":        "1",
										"firstName": followerName,
									},
								},
							},
						},
					},
				},
			},
			map[string]interface{}{
				"firstName": "goodbye",
				"friends": []interface{}{
					map[string]interface{}{
						"firstName": "Jingleheymer",
						"id":        "3",
						"photoGallery": []interface{}{
							map[string]interface{}{
								"url": photoGalleryURL,
								"followers": []interface{}{
									map[string]interface{}{
										"id":        "1",
										"firstName": followerName,
									},
								},
							},
						},
					},
					map[string]interface{}{
						"firstName": "Schmidt",
						"id":        "4",
						"photoGallery": []interface{}{
							map[string]interface{}{
								"url": photoGalleryURL,
								"followers": []interface{}{
									map[string]interface{}{
										"id":        "1",
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

func TestExecutor_multipleErrors(t *testing.T) {
	// an executor should return a list of every error that it encounters while executing the plan

	// build a query plan that the executor will follow
	_, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					{
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
						Queryer: graphql.QueryerFunc(
							func(input *graphql.QueryInput) (interface{}, error) {
								return map[string]interface{}{"data": map[string]interface{}{}}, errors.New("message")
							},
						),
					},
					{
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
						Queryer: graphql.QueryerFunc(
							func(input *graphql.QueryInput) (interface{}, error) {
								return map[string]interface{}{"data": map[string]interface{}{}}, graphql.ErrorList{errors.New("message"), errors.New("message")}
							},
						),
					},
				},
			},
		},
	})
	if err == nil {
		t.Errorf("Did not encounter error executing plan")
		return
	}

	// since 3 errors were thrown we need to make sure we actually received an error list
	list, ok := err.(graphql.ErrorList)
	if !ok {
		t.Error("Error was not an error list")
		return
	}

	if !assert.Len(t, list, 3, "Error list did not have 3 items") {
		return
	}
}

func TestExecutor_includeIf(t *testing.T) {

	// the query we want to execute is
	// {
	// 		user @include(if: false) {   <- from serviceA
	// 			favoriteCatPhoto {   	 <- from serviceB
	// 				url              	 <- from serviceB
	// 			}
	// 		}
	// }

	// build a query plan that the executor will follow
	result, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Plan: &QueryPlan{
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					{

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
								Directives: ast.DirectiveList{
									&ast.Directive{
										Name: "include",
										Arguments: ast.ArgumentList{
											&ast.Argument{
												Name: "if",
												Value: &ast.Value{
													Kind: ast.BooleanValue,
													Raw:  "true",
												},
											},
										},
									},
								},
							},
						},
						// return a known value we can test against
						Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{}},
						// then we have to ask for the users favorite cat photo and its url
						Then: []*QueryPlanStep{
							{
								ParentType:     "User",
								InsertionPoint: []string{"user"},
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
								Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
									"node": map[string]interface{}{
										"favoriteCatPhoto": map[string]interface{}{
											"url": "hello world",
										},
									},
								}},
							},
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

	assert.Equal(t, map[string]interface{}{}, result)
}

func TestExecutor_appliesRequestMiddlewares(t *testing.T) {
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

	// the middleware to apply
	called := false
	middleware := RequestMiddleware(func(r *http.Request) error {
		called = true
		return nil
	})

	// in order to execute the request middleware we need to be dealing with a network queryer
	// which means we need to fake out the http client
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			// serialize the json we want to send back
			result, _ := json.Marshal(map[string]interface{}{
				"allUsers": []string{
					"John Jacob",
					"Jinglehymer Schmidt",
				},
			})

			return &http.Response{
				StatusCode: 200,
				// Send response to be tested
				Body: ioutil.NopCloser(bytes.NewBuffer(result)),
				// Must be set to non-nil value or it panics
				Header: make(http.Header),
			}
		}),
	}

	// we need a planner that will leave behind a simple plan
	planner := &MockPlanner{
		QueryPlanList{
			{
				Operation: &ast.OperationDefinition{
					Operation: ast.Query,
				},
				RootStep: &QueryPlanStep{
					Then: []*QueryPlanStep{
						{
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
							QueryDocument: &ast.QueryDocument{
								Operations: ast.OperationList{
									{
										Operation: "Query",
									},
								},
							},
							QueryString: `hello`,
							// return a known value we can test against
							Queryer: graphql.NewSingleRequestQueryer("hello").WithHTTPClient(httpClient),
						},
					},
				},
			},
		},
	}

	// create a gateway with the Middleware
	gateway, err := New([]*graphql.RemoteSchema{remoteSchema}, WithMiddlewares(middleware), WithPlanner(planner))
	if err != nil {
		t.Error(err.Error())
		return
	}

	reqCtx := &RequestContext{
		Context: context.Background(),
		Query:   "{ values }",
	}
	plan, _ := gateway.GetPlans(reqCtx)

	// execute any think
	gateway.Execute(reqCtx, plan)

	// make sure we called the middleware
	assert.True(t, called, "Did not call middleware")
}

func TestExecutor_threadsVariables(t *testing.T) {
	// the variables we'll be threading through
	fullVariables := map[string]interface{}{
		"hello":   "world",
		"goodbye": "moon",
	}

	// the precompiled query document coming in
	fullVariableDefs := ast.VariableDefinitionList{
		&ast.VariableDefinition{
			Variable: "hello",
			Type:     ast.NamedType("ID", &ast.Position{}),
		},
		&ast.VariableDefinition{
			Variable: "goodbye",
			Type:     ast.NamedType("ID", &ast.Position{}),
		},
	}

	// build a query plan that the executor will follow
	_, err := (&ParallelExecutor{}).Execute(&ExecutionContext{
		RequestContext: context.Background(),
		Variables:      fullVariables,
		Plan: &QueryPlan{
			Operation: &ast.OperationDefinition{
				Operation:           ast.Query,
				Name:                "hoopla",
				VariableDefinitions: fullVariableDefs,
			},
			RootStep: &QueryPlanStep{
				Then: []*QueryPlanStep{
					{
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
						QueryDocument: &ast.QueryDocument{
							Operations: ast.OperationList{
								{
									Operation:           "Query",
									VariableDefinitions: ast.VariableDefinitionList{fullVariableDefs[0]},
								},
							},
						},
						QueryString: `hello`,
						Variables:   Set{"hello": true},
						// return a known value we can test against
						Queryer: graphql.QueryerFunc(
							func(input *graphql.QueryInput) (interface{}, error) {
								// make sure that we got the right variable inputs
								assert.Equal(t, map[string]interface{}{"hello": "world"}, input.Variables)
								// and definitions
								assert.Equal(t, ast.VariableDefinitionList{fullVariableDefs[0]}, input.QueryDocument.Operations[0].VariableDefinitions)
								assert.Equal(t, "hello", input.Query)
								assert.Equal(t, "hoopla", input.OperationName)

								return map[string]interface{}{"values": []string{"world"}}, nil
							},
						),
					},
				},
			},
		},
	})
	if err != nil {
		t.Error(err.Error())
	}
}
func TestFindInsertionPoint_rootList(t *testing.T) {
	// in this example, the step before would have just resolved (need to be inserted at)
	// ["users", "photoGallery"]. There would be an id field underneath each photo in the list
	// of users.photoGallery

	// we want the list of insertion points that point to
	planInsertionPoint := []string{"users", "photoGallery", "likedBy"}

	// pretend we are in the middle of stitching a larger object
	startingPoint := [][]string{}

	// there are 6 total insertion points in this example
	finalInsertionPoint := [][]string{
		// photo 0 is liked by 2 users
		{"users:0", "photoGallery:0", "likedBy:0#1"},
		{"users:0", "photoGallery:0", "likedBy:1#2"},
		// photo 1 is liked by 3 users
		{"users:0", "photoGallery:1", "likedBy:0#3"},
		{"users:0", "photoGallery:1", "likedBy:1#4"},
		{"users:0", "photoGallery:1", "likedBy:2#5"},
		// photo 2 is liked by 1 user
		{"users:0", "photoGallery:2", "likedBy:0#6"},
	}

	// the selection we're going to make
	stepSelectionSet := ast.SelectionSet{
		&ast.Field{
			Name: "users",
			Definition: &ast.FieldDefinition{
				Type: ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
			},
			SelectionSet: ast.SelectionSet{
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
			},
		},
	}

	// the result of the step
	result := map[string]interface{}{
		"users": []interface{}{
			map[string]interface{}{
				"photoGallery": []interface{}{
					map[string]interface{}{
						"likedBy": []interface{}{
							map[string]interface{}{
								"totalLikes": 10,
								"id":         "1",
							},
							map[string]interface{}{
								"totalLikes": 10,
								"id":         "2",
							},
						},
					},
					map[string]interface{}{
						"likedBy": []interface{}{
							map[string]interface{}{
								"totalLikes": 10,
								"id":         "3",
							},
							map[string]interface{}{
								"totalLikes": 10,
								"id":         "4",
							},
							map[string]interface{}{
								"totalLikes": 10,
								"id":         "5",
							},
						},
					},
					map[string]interface{}{
						"likedBy": []interface{}{
							map[string]interface{}{
								"totalLikes": 10,
								"id":         "6",
							},
						},
					},
					map[string]interface{}{
						"likedBy": []interface{}{},
					},
				},
			},
		},
	}

	generatedPoint, err := executorFindInsertionPoints(&sync.Mutex{}, planInsertionPoint, stepSelectionSet, result, startingPoint, nil)
	if err != nil {
		t.Error(t, err)
		return
	}

	assert.Equal(t, finalInsertionPoint, generatedPoint)
}

func TestFindObject(t *testing.T) {
	// create an object we want to extract
	source := map[string]interface{}{
		"hello": []interface{}{
			map[string]interface{}{
				"firstName": "0",
				"friends": []interface{}{
					map[string]interface{}{
						"firstName": "2",
						"friends": []interface{}{
							map[string]interface{}{
								"firstName": "Hello1",
							},
						},
					},
					map[string]interface{}{
						"firstName": "3",
						"friends": []interface{}{
							map[string]interface{}{
								"firstName": "Hello2",
							},
						},
					},
				},
			},
			map[string]interface{}{
				"firstName": "4",
				"friends": []interface{}{
					map[string]interface{}{
						"firstName": "5",
						"friends": []interface{}{
							map[string]interface{}{
								"firstName": "Hello3",
							},
						},
					},
					map[string]interface{}{
						"firstName": "6",
						"friends": []interface{}{
							map[string]interface{}{
								"firstName": "Hello4",
							},
						},
					},
				},
			},
		},
	}

	value, err := executorExtractValue(source, &sync.Mutex{}, []string{"hello:0", "friends:1", "friends:0"})
	if err != nil {
		t.Error(err.Error())
		return
	}

	assert.Equal(t, map[string]interface{}{
		"firstName": "Hello2",
	}, value)
}

func TestFindString(t *testing.T) {
	// create an object we want to extract
	source := map[string]interface{}{
		"hello": []interface{}{
			map[string]interface{}{
				"firstName": "0",
				"friends": []interface{}{
					map[string]interface{}{
						"firstName": "2",
					},
					map[string]interface{}{
						"firstName": "3",
					},
				},
			},
			map[string]interface{}{
				"firstName": "4",
				"friends": []interface{}{
					map[string]interface{}{
						"firstName": "5",
					},
					map[string]interface{}{
						"firstName": "6",
					},
				},
			},
		},
	}

	value, err := executorExtractValue(source, &sync.Mutex{}, []string{"hello:0", "friends:1", "firstName"})
	if err != nil {
		t.Error(err.Error())
		return
	}

	assert.Equal(t, "3", value)
}

func TestExecutorInsertObject_insertObjectValues(t *testing.T) {
	// the object to mutate
	source := map[string]interface{}{}

	// the object to insert
	inserted := map[string]interface{}{"hello": "world"}

	// insert the string deeeeep down
	err := executorInsertObject(source, &sync.Mutex{}, []string{"hello:5#1", "message", "body:2"}, inserted)
	if err != nil {
		t.Error(err)
		return
	}

	// there should be a list under the key "hello"
	rootList, ok := source["hello"]
	if !ok {
		t.Error("Did not add root list")
		return
	}
	list, ok := rootList.([]interface{})
	if !ok {
		t.Error("root list is not a list")
		return
	}

	if len(list) != 6 {
		t.Errorf("Root list did not have enough entries.")
		assert.Equal(t, 6, len(list))
		return
	}

	entry, ok := list[5].(map[string]interface{})
	if !ok {
		t.Error("6th entry wasn't an object")
		return
	}

	// the object we care about is index 5
	message := entry["message"]
	if message == nil {
		t.Error("Did not add message to object")
		return
	}

	msgObj, ok := message.(map[string]interface{})
	if !ok {
		t.Error("message is not a list")
		return
	}

	// there should be a list under it called body
	bodiesList, ok := msgObj["body"]
	if !ok {
		t.Error("Did not add body list")
		return
	}
	bodies, ok := bodiesList.([]interface{})
	if !ok {
		t.Error("bodies list is not a list")
		return
	}

	if len(bodies) != 3 {
		t.Error("bodies list did not have enough entries")
		return
	}
	body, ok := bodies[2].(map[string]interface{})
	if !ok {
		t.Error("Body was not an object")
		return
	}

	// make sure that the value is what we expect
	assert.Equal(t, inserted, body)
}

func TestExecutorInsertObject_insertListElements(t *testing.T) {
	// the object to mutate
	source := map[string]interface{}{}

	// the object to insert
	inserted := map[string]interface{}{
		"hello": "world",
	}

	// insert the object deeeeep down
	err := executorInsertObject(source, &sync.Mutex{}, []string{"hello", "objects:5"}, inserted)
	if err != nil {
		t.Error(err)
		return
	}

	// there should be an object under the key "hello"
	rootEntry, ok := source["hello"]
	if !ok {
		t.Error("Did not add root entry")
		return
	}

	root, ok := rootEntry.(map[string]interface{})
	if !ok {
		t.Error("root object is not an object")
		return
	}

	rootList, ok := root["objects"]
	if !ok {
		t.Error("did not add objects list")
		return
	}

	list, ok := rootList.([]interface{})
	if !ok {
		t.Error("objects is not a list")
		return
	}

	if len(list) != 6 {
		t.Errorf("Root list did not have enough entries.")
		assert.Equal(t, 6, len(list))
		return
	}

	// make sure that the value is what we expect
	assert.Equal(t, inserted, list[5])
}

func TestExecutorGetPointData(t *testing.T) {
	table := []struct {
		point string
		data  *extractorPointData
	}{
		{"foo:2", &extractorPointData{Field: "foo", Index: 2, ID: ""}},
		{"foo#3", &extractorPointData{Field: "foo", Index: -1, ID: "3"}},
		{"foo:2#3", &extractorPointData{Field: "foo", Index: 2, ID: "3"}},
	}

	for _, row := range table {
		t.Run(row.point, func(t *testing.T) {
			pointData, err := executorGetPointData(row.point)
			if err != nil {
				t.Error(err.Error())
				return
			}

			assert.Equal(t, row.data, pointData)
		})
	}
}

func TestFindInsertionPoint_bailOnNil(t *testing.T) {
	// we want the list of insertion points that point to
	planInsertionPoint := []string{"post", "author"}
	expected := [][]string{}

	result := map[string]interface{}{
		"post": map[string]interface{}{
			"author": nil,
		},
	}

	// the selection we're going to make
	stepSelectionSet := ast.SelectionSet{
		&ast.Field{
			Name: "user",
			Definition: &ast.FieldDefinition{
				Type: ast.ListType(ast.NamedType("Photo", &ast.Position{}), &ast.Position{}),
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
	}

	generatedPoint, err := executorFindInsertionPoints(&sync.Mutex{}, planInsertionPoint, stepSelectionSet, result, [][]string{}, nil)
	if err != nil {
		t.Error(t, err)
		return
	}

	assert.Equal(t, expected, generatedPoint)
}

func TestFindInsertionPoint_stitchIntoObject(t *testing.T) {
	// we want the list of insertion points that point to
	planInsertionPoint := []string{"users", "photoGallery", "author"}

	// pretend we are in the middle of stitching a larger object
	startingPoint := [][]string{{"users:0"}}

	// there are 3 total insertion points in this example
	finalInsertionPoint := [][]string{
		{"users:0", "photoGallery:0", "author#1"},
		{"users:0", "photoGallery:1", "author#2"},
		{"users:0", "photoGallery:2", "author#3"},
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
	result := map[string]interface{}{
		"photoGallery": []interface{}{
			map[string]interface{}{
				"author": map[string]interface{}{
					"id": "1",
				},
			},
			map[string]interface{}{
				"author": map[string]interface{}{
					"id": "2",
				},
			},
			map[string]interface{}{
				"author": map[string]interface{}{
					"id": "3",
				},
			},
		},
	}

	generatedPoint, err := executorFindInsertionPoints(&sync.Mutex{}, planInsertionPoint, stepSelectionSet, result, startingPoint, nil)
	if err != nil {
		t.Error(t, err)
		return
	}

	assert.Equal(t, finalInsertionPoint, generatedPoint)
}

func TestFindInsertionPoint_handlesNullObjects(t *testing.T) {
	t.Skip("Not yet implemented")
}
