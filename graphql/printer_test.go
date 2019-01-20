package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

func TestPrintQuery(t *testing.T) {
	table := []struct {
		expected string
		query    *ast.QueryDocument
	}{
		// single root field
		{
			`{
  hello
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{
					&ast.OperationDefinition{
						Operation: ast.Query,
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Name: "hello",
							},
						},
					},
				},
			},
		},
		// variable values
		{
			`{
  hello(foo: $foo)
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{
					&ast.OperationDefinition{
						Operation: ast.Query,
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Name: "hello",
								Arguments: ast.ArgumentList{
									&ast.Argument{
										Name: "foo",
										Value: &ast.Value{
											Kind: ast.Variable,
											Raw:  "foo",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		// directives
		{
			`{
  hello @foo(bar: "baz")
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							Directives: ast.DirectiveList{
								&ast.Directive{
									Name: "foo",
									Arguments: ast.ArgumentList{
										&ast.Argument{
											Name: "bar",
											Value: &ast.Value{
												Kind: ast.StringValue,
												Raw:  "baz",
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
		{
			`{
  ... on User @foo {
    hello
  }
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{
					&ast.OperationDefinition{
						Operation: ast.Query,
						SelectionSet: ast.SelectionSet{
							&ast.InlineFragment{
								TypeCondition: "User",
								SelectionSet: ast.SelectionSet{
									&ast.Field{
										Name: "hello",
									},
								},
								Directives: ast.DirectiveList{
									&ast.Directive{
										Name: "foo",
									},
								},
							},
						},
					},
				},
			},
		},
		// multiple root fields
		{
			`{
  hello
  goodbye
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{
					&ast.OperationDefinition{
						Operation: ast.Query,
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Name: "hello",
							},
							&ast.Field{
								Name: "goodbye",
							},
						},
					},
				},
			},
		},
		// selection set
		{
			`{
  hello {
    world
  }
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							SelectionSet: ast.SelectionSet{
								&ast.Field{
									Name: "world",
								},
							},
						},
					},
				},
				},
			},
		},
		// inline fragments
		{
			`{
  ... on Foo {
    hello
  }
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{
					&ast.OperationDefinition{
						Operation: ast.Query,
						SelectionSet: ast.SelectionSet{
							&ast.InlineFragment{
								TypeCondition: "Foo",
								SelectionSet: ast.SelectionSet{
									&ast.Field{
										Name: "hello",
									},
								},
							},
						},
					},
				},
			},
		},
		// fragments
		{
			`{
  ...Foo
}

fragment Foo on User {
  firstName
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{
					&ast.OperationDefinition{
						Operation: ast.Query,
						SelectionSet: ast.SelectionSet{
							&ast.FragmentSpread{
								Name: "Foo",
							},
						},
					},
				},
				Fragments: ast.FragmentDefinitionList{
					&ast.FragmentDefinition{
						Name: "Foo",
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Name: "firstName",
								Definition: &ast.FieldDefinition{
									Type: ast.NamedType("String", &ast.Position{}),
								},
							},
						},
						TypeCondition: "User",
					},
				},
			},
		},
		// alias
		{
			`{
  bar: hello
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name:  "hello",
							Alias: "bar",
						},
					},
				},
				},
			},
		},
		// string arguments
		{
			`{
  hello(hello: "world")
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							Arguments: ast.ArgumentList{
								{
									Name: "hello",
									Value: &ast.Value{
										Kind: ast.StringValue,
										Raw:  "world",
									},
								},
							},
						},
					},
				},
				},
			},
		},
		// int arguments
		{
			`{
  hello(hello: 1)
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							Arguments: ast.ArgumentList{
								{
									Name: "hello",
									Value: &ast.Value{
										Kind: ast.IntValue,
										Raw:  "1",
									},
								},
							},
						},
					},
				},
				},
			},
		},
		// boolean arguments
		{
			`{
  hello(hello: true)
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							Arguments: ast.ArgumentList{
								{
									Name: "hello",
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
			},
		},
		// variable arguments
		{
			`{
  hello(hello: $hello)
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							Arguments: ast.ArgumentList{
								{
									Name: "hello",
									Value: &ast.Value{
										Kind: ast.IntValue,
										Raw:  "$hello",
									},
								},
							},
						},
					},
				},
				},
			},
		},
		// null arguments
		{
			`{
  hello(hello: null)
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							Arguments: ast.ArgumentList{
								{
									Name: "hello",
									Value: &ast.Value{
										Kind: ast.NullValue,
									},
								},
							},
						},
					},
				},
				},
			},
		},
		// float arguments
		{
			`{
  hello(hello: 1.1)
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							Arguments: ast.ArgumentList{
								{
									Name: "hello",
									Value: &ast.Value{
										Kind: ast.FloatValue,
										Raw:  "1.1",
									},
								},
							},
						},
					},
				},
				},
			},
		},
		// enum arguments
		{
			`{
  hello(hello: Hello)
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							Arguments: ast.ArgumentList{
								{
									Name: "hello",
									Value: &ast.Value{
										Kind: ast.EnumValue,
										Raw:  "Hello",
									},
								},
							},
						},
					},
				},
				},
			},
		},
		// list arguments
		{
			`{
  hello(hello: ["hello", 1])
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							Arguments: ast.ArgumentList{
								{
									Name: "hello",
									Value: &ast.Value{
										Kind: ast.ListValue,
										Children: ast.ChildValueList{
											{
												Value: &ast.Value{
													Kind: ast.StringValue,
													Raw:  "hello",
												},
											},
											{
												Value: &ast.Value{
													Kind: ast.IntValue,
													Raw:  "1",
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
		},
		// object arguments
		{
			`{
  hello(hello: {hello: "hello", goodbye: 1})
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							Arguments: ast.ArgumentList{
								{
									Name: "hello",
									Value: &ast.Value{
										Kind: ast.ObjectValue,
										Children: ast.ChildValueList{
											{
												Name: "hello",
												Value: &ast.Value{
													Kind: ast.StringValue,
													Raw:  "hello",
												},
											},
											{
												Name: "goodbye",
												Value: &ast.Value{
													Kind: ast.IntValue,
													Raw:  "1",
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
		},
		// multiple arguments
		{
			`{
  hello(hello: "world", goodbye: "moon")
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
							Arguments: ast.ArgumentList{
								{
									Name: "hello",
									Value: &ast.Value{
										Kind: ast.StringValue,
										Raw:  "world",
									},
								},
								{
									Name: "goodbye",
									Value: &ast.Value{
										Kind: ast.StringValue,
										Raw:  "moon",
									},
								},
							},
						},
					},
				},
				},
			},
		},
		// anonymous variables to query
		{
			`query ($id: ID!) {
  hello
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
						},
					},
					VariableDefinitions: ast.VariableDefinitionList{
						&ast.VariableDefinition{
							Variable: "id",
							Type: &ast.Type{
								NamedType: "ID",
								NonNull:   true,
							},
						},
					},
				},
				},
			},
		},
		// named query with variables
		{
			`query foo($id: ID!) {
  hello
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Query,
					Name:      "foo",
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
						},
					},
					VariableDefinitions: ast.VariableDefinitionList{
						&ast.VariableDefinition{
							Variable: "id",
							Type: &ast.Type{
								NamedType: "ID",
								NonNull:   true,
							},
						},
					},
				},
				},
			},
		},
		// single mutation field
		{
			`mutation {
  hello
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{&ast.OperationDefinition{
					Operation: ast.Mutation,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "hello",
						},
					},
				},
				},
			},
		},
		// single subscription field
		{
			`subscription {
  hello
}
`,
			&ast.QueryDocument{
				Operations: ast.OperationList{
					&ast.OperationDefinition{
						Operation: ast.Subscription,
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Name: "hello",
							},
						},
					},
				},
			},
		},
	}

	for _, row := range table {
		str, err := PrintQuery(row.query)
		if err != nil {
			t.Error(err.Error())
		}

		assert.Equal(t, row.expected, str)
	}
}
