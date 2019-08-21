package gateway

import (
	"testing"

	"github.com/nautilus/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

func TestInMemoryQueryPersister(t *testing.T) {
	// the query plan we are going to persist
	plan := &QueryPlan{
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
					Variables:   Set{"hello": true},
					// return a known value we can test against
					Queryer: graphql.QueryerFunc(
						func(input *graphql.QueryInput) (interface{}, error) {
							// make sure that we got the right variable inputs
							assert.Equal(t, map[string]interface{}{"hello": "world"}, input.Variables)
							// and definitions
							assert.Equal(t, ast.VariableDefinitionList{fullVariableDefs[0]}, input.QueryDocument.Operations[0].VariableDefinitions)
							assert.Equal(t, "hello", input.Query)

							return map[string]interface{}{"values": []string{"world"}}, nil
						},
					),
				},
			},
		},
	}
}
