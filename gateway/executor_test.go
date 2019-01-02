package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

func TestExecutor_plansOfOne(t *testing.T) {
	// build a query plan that the executor will follow
	result, err := (&ParallelExecutor{}).Execute(&QueryPlan{
		RootStep: &QueryPlanStep{
			// this is equivalent to
			// query { values }
			ParentType: "Query",
			SelectionSet: ast.SelectionSet{
				&ast.Field{
					Name: "values",
				},
			},
			// return a known value we can test against
			Queryer: &MockQueryer{map[string]interface{}{
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
			// query { values }
			ParentType:     "Query",
			InsertionPoint: []string{},
			SelectionSet: ast.SelectionSet{
				&ast.Field{
					Name: "user",
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "firstName",
						},
					},
				},
			},
			// return a known value we can test against
			Queryer: &MockQueryer{map[string]interface{}{
				"user": map[string]interface{}{
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
							SelectionSet: ast.SelectionSet{
								&ast.Field{
									Name: "url",
								},
							},
						},
					},
					Queryer: &MockQueryer{map[string]interface{}{
						"node": map[string]interface{}{
							"favoriteCatPhoto": map[string]interface{}{
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
	}

	// make sure we got the right values back
	assert.Equal(t, map[string]interface{}{
		"user": map[string]interface{}{
			"firstName": "hello",
			"favoriteCatPhoto": map[string]interface{}{
				"url": "hello world",
			},
		},
	}, result)
}
