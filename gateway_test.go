package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nautilus/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

type schemaTableRow struct {
	location string
	query    string
}

func TestGateway(t *testing.T) {
	t.Parallel()
	schemas := []schemaTableRow{
		{
			"url1",
			`
				type Query {
					allUsers: [User!]!
				}

				type User {
					firstName: String!
					lastName: String!
				}
			`,
		},
		{
			"url2",
			`
				type User {
					lastName: String!
				}
			`,
		},
	}

	// the list of remote schemas
	sources := []*graphql.RemoteSchema{}

	for _, source := range schemas {
		// turn the combo into a remote schema
		schema, _ := graphql.LoadSchema(source.query)

		// add the schema to list of sources
		sources = append(sources, &graphql.RemoteSchema{Schema: schema, URL: source.location})
	}

	t.Run("Compute Field URLs", func(t *testing.T) {
		t.Parallel()
		locations := fieldURLs(sources, false)

		allUsersURL, err := locations.URLFor(typeNameQuery, "allUsers")
		assert.Nil(t, err)
		assert.Equal(t, []string{"url1"}, allUsersURL)

		lastNameURL, err := locations.URLFor("User", "lastName")
		assert.Nil(t, err)
		assert.Equal(t, []string{"url1", "url2"}, lastNameURL)

		firstNameURL, err := locations.URLFor("User", "firstName")
		assert.Nil(t, err)
		assert.Equal(t, []string{"url1"}, firstNameURL)

		// make sure we can look up the url for internal
		_, ok := locations["__Schema.types"]
		if !ok {
			t.Error("Could not find internal type __Schema.types")
			return
		}

		_, ok = locations["Query.__schema"]
		if !ok {
			t.Error("Could not find internal field Query.__schema")
			return
		}
	})

	t.Run("Options", func(t *testing.T) {
		t.Parallel()
		// create a new schema with the sources and some configuration
		gateway, err := New([]*graphql.RemoteSchema{sources[0]}, func(schema *Gateway) {
			schema.sources = append(schema.sources, sources[1])
		})
		if err != nil {
			t.Error(err.Error())
			return
		}

		// make sure that the schema has both sources
		assert.Len(t, gateway.sources, 2)
	})

	t.Run("WithPlanner", func(t *testing.T) {
		t.Parallel()
		// the planner we will assign
		planner := &MockPlanner{}

		gateway, err := New(sources, WithPlanner(planner))
		if err != nil {
			t.Error(err.Error())
			return
		}

		assert.Equal(t, planner, gateway.planner)
	})

	t.Run("WithQueryerFactory", func(t *testing.T) {
		t.Parallel()
		// the planner we will assign
		planner := &MinQueriesPlanner{}

		factory := QueryerFactory(func(ctx *PlanningContext, url string) graphql.Queryer {
			return ctx.Gateway
		})

		// instantiate the gateway
		gateway, err := New(sources, WithPlanner(planner), WithQueryerFactory(&factory))
		if err != nil {
			t.Error(err.Error())
			return
		}

		assert.Equal(t, &factory, gateway.planner.(*MinQueriesPlanner).QueryerFactory)
	})

	t.Run("WithLocationPriorities", func(t *testing.T) {
		t.Parallel()
		priorities := []string{"url1", "url2"}

		gateway, err := New(sources, WithLocationPriorities(priorities))
		if err != nil {
			t.Error(err.Error())
			return
		}

		assert.Equal(t, priorities, gateway.locationPriorities)
	})

	t.Run("WithLogger", func(t *testing.T) {
		t.Parallel()
		logger := &DefaultLogger{}

		go func() { // verify no race condition between Gateway instances (singleton): https://github.com/nautilus/gateway/issues/154
			_, err := New(sources, WithLogger(logger))
			if err != nil {
				t.Error(err.Error())
			}
		}()

		g, err := New(sources, WithLogger(logger))
		if err != nil {
			t.Error(err.Error())
			return
		}

		assert.Equal(t, logger, g.logger)
	})

	t.Run("fieldURLs ignore introspection", func(t *testing.T) {
		t.Parallel()
		locations := fieldURLs(sources, true)

		for key := range locations {
			if strings.HasPrefix(key, "__") {
				t.Errorf("Found type starting with __: %s", key)
			}
		}

		if _, ok := locations["Query.__schema"]; ok {
			t.Error("Encountered introspection value Query.__schema")
			return
		}
	})

	t.Run("Response Middleware Error", func(t *testing.T) {
		t.Parallel()
		// create a new schema with the sources and some configuration
		gateway, err := New(sources,
			WithExecutor(ExecutorFunc(func(ctx *ExecutionContext) (map[string]interface{}, error) {
				return map[string]interface{}{"goodbye": "moon"}, nil
			})),
			WithMiddlewares(
				ResponseMiddleware(func(ctx *ExecutionContext, response map[string]interface{}) error {
					return errors.New("this string")
				}),
			))
		if err != nil {
			t.Error(err.Error())
			return
		}

		// build a query plan that the executor will follow
		reqCtx := &RequestContext{
			Context: context.Background(),
			Query:   "{ allUsers { firstName } }",
		}
		plans, err := gateway.GetPlans(reqCtx)
		if err != nil {
			t.Errorf("Encountered error building plan.")
		}

		_, err = gateway.Execute(reqCtx, plans)
		if err == nil {
			t.Errorf("Did not encounter error executing plan.")
		}
	})

	t.Run("Response Middleware Error Empty Data", func(t *testing.T) {
		t.Parallel()
		// create a new schema with the sources and some configuration
		gateway, err := New(sources,
			WithExecutor(ExecutorFunc(func(ctx *ExecutionContext) (map[string]interface{}, error) {
				return map[string]interface{}{}, errors.New("error string")
			})),
			WithMiddlewares(
				ResponseMiddleware(func(ctx *ExecutionContext, response map[string]interface{}) error {
					return errors.New("this string")
				}),
			))
		if err != nil {
			t.Error(err.Error())
			return
		}

		// build a query plan that the executor will follow
		reqCtx := &RequestContext{
			Context: context.Background(),
			Query:   "{ allUsers { firstName } }",
		}
		plans, err := gateway.GetPlans(reqCtx)
		if err != nil {
			t.Errorf("Encountered error building plan.")
		}

		res, err := gateway.Execute(reqCtx, plans)
		if err == nil {
			t.Errorf("Did not encounter error executing plan.")
		}
		assert.Nil(t, res)
	})

	t.Run("Response Middleware Success", func(t *testing.T) {
		t.Parallel()
		// create a new schema with the sources and some configuration
		gateway, err := New(sources,
			WithExecutor(ExecutorFunc(func(ctx *ExecutionContext) (map[string]interface{}, error) {
				return map[string]interface{}{"goodbye": "moon"}, nil
			})),
			WithMiddlewares(
				ResponseMiddleware(func(ctx *ExecutionContext, response map[string]interface{}) error {
					// clear the previous value
					for k := range response {
						delete(response, k)
					}

					// set something we can test against
					response["hello"] = "world"

					// no errors
					return nil
				}),
			))
		if err != nil {
			t.Error(err.Error())
			return
		}
		reqCtx := &RequestContext{
			Context: context.Background(),
			Query:   "{ allUsers { firstName } }",
		}

		plan, err := gateway.GetPlans(reqCtx)
		if err != nil {
			t.Errorf("Encountered error building plan: %s", err.Error())
			return
		}

		// build a query plan that the executor will follow
		response, err := gateway.Execute(reqCtx, plan)
		if err != nil {
			t.Errorf("Encountered error executing plan: %s", err.Error())
			return
		}
		// make sure our middleware changed the response
		assert.Equal(t, map[string]interface{}{"hello": "world"}, response)
	})

	t.Run("filter out automatically inserted ids", func(t *testing.T) {
		t.Parallel()
		// the query we're going to fire. Query.allUsers comes from service one. User.lastName
		// from service two.
		query := `
			{
				allUsers {
					lastName
				}
			}
		`

		// create a new schema with the sources and a planner that will respond with
		// values that have ids
		gateway, err := New(sources, WithPlanner(&MockPlanner{
			QueryPlanList{
				&QueryPlan{
					FieldsToScrub: map[string][][]string{
						"id": {
							{"allUsers"},
						},
					},
					Operation: &ast.OperationDefinition{
						Operation: ast.Query,
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Name: "allUsers",
								Definition: &ast.FieldDefinition{
									Type: ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
								},
							},
						},
					},
					RootStep: &QueryPlanStep{
						Then: []*QueryPlanStep{
							{
								// this is equivalent to
								// query { allUsers }
								ParentType:     typeNameQuery,
								InsertionPoint: []string{},
								SelectionSet: ast.SelectionSet{
									&ast.Field{
										Name: "allUsers",
										Definition: &ast.FieldDefinition{
											Type: ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
										},
										SelectionSet: ast.SelectionSet{
											&ast.Field{
												Name: "id",
												Definition: &ast.FieldDefinition{
													Type: ast.NamedType("ID", &ast.Position{}),
												},
											},
										},
									},
								},
								// return a known value we can test against
								Queryer: &graphql.MockSuccessQueryer{Value: map[string]interface{}{
									"allUsers": []interface{}{
										map[string]interface{}{
											"id": "1",
										},
									},
								}},
								// then we have to ask for the users favorite cat photo and its url
								Then: []*QueryPlanStep{
									{
										ParentType:     "User",
										InsertionPoint: []string{"allUsers"},
										SelectionSet: ast.SelectionSet{
											&ast.Field{
												Name: "lastName",
												Definition: &ast.FieldDefinition{
													Type: ast.NamedType("String", &ast.Position{}),
												},
											},
										},
										Queryer: &graphql.MockSuccessQueryer{Value: map[string]interface{}{
											"node": map[string]interface{}{
												"lastName": "Hello",
											},
										}},
									},
								},
							},
						},
					},
				},
			},
		}))
		if err != nil {
			t.Error(err.Error())
			return
		}

		reqCtx := &RequestContext{
			Context: context.Background(), Query: query,
		}
		plan, err := gateway.GetPlans(reqCtx)
		if err != nil {
			t.Error(err.Error())
			return
		}

		// execute the query
		res, err := gateway.Execute(reqCtx, plan)
		if err != nil {
			t.Error(err.Error())
			return
		}

		// make sure we didn't get any ids
		assert.Equal(t, map[string]interface{}{
			"allUsers": []interface{}{
				map[string]interface{}{
					"lastName": "Hello",
				},
			},
		}, res)
	})

	t.Run("Introspection field on services", func(t *testing.T) {
		t.Parallel()
		// compute the location of each field
		locations := fieldURLs(sources, false)

		// make sure we have entries for __typename at each service
		userTypenameURLs, err := locations.URLFor("User", "__typename")
		assert.Nil(t, err)
		assert.Equal(t, []string{"url1", "url2"}, userTypenameURLs)
	})

	t.Run("Gateway fields", func(t *testing.T) {
		t.Parallel()
		// define a gateway field
		viewerField := &QueryField{
			Name: "viewer",
			Type: ast.NamedType("User", &ast.Position{}),
			Arguments: ast.ArgumentDefinitionList{
				&ast.ArgumentDefinition{
					Name: "id",
					Type: ast.NamedType("ID", &ast.Position{}),
				},
			},
			Resolver: func(ctx context.Context, args map[string]interface{}) (string, error) {
				return args["id"].(string), nil
			},
		}

		// create a gateway with the viewer field
		gateway, err := New(sources, WithQueryFields(viewerField))
		assert.NoError(t, err)

		// execute the query
		query := `
			query($id: ID!){
				viewer(id: $id) {
					firstName
				}
			}
		`
		plans, err := gateway.planner.Plan(&PlanningContext{
			Query:     query,
			Locations: gateway.fieldURLs,
			Schema:    gateway.schema,
			Gateway:   gateway,
		})
		if err != nil {
			t.Error(err.Error())
			return
		}

		if !assert.Len(t, plans[0].RootStep.Then, 1) {
			return
		}

		// invoke the first step
		res := map[string]interface{}{}
		err = plans[0].RootStep.Then[0].Queryer.Query(context.Background(), &graphql.QueryInput{
			Query: query,
			QueryDocument: &ast.QueryDocument{
				Operations: ast.OperationList{
					{
						Operation: typeNameQuery,
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Alias: "viewer",
								Name:  "viewer",
								Arguments: ast.ArgumentList{
									&ast.Argument{
										Name: "id",
										Value: &ast.Value{
											Kind: ast.Variable,
											Raw:  "id",
										},
									},
								},
							},
						},
					},
				},
			},
			Variables: map[string]interface{}{"id": "1"},
		}, &res)
		if err != nil {
			t.Error(err.Error())
			return
		}

		// make sure the result of the queryer matches exepctations
		assert.Equal(t, map[string]interface{}{"viewer": map[string]interface{}{"id": "1"}}, res)
	})
}

func TestGatewayExecuteRespectsOperationName(t *testing.T) {
	t.Parallel()
	// define a schema source
	schema, _ := graphql.LoadSchema(`
		type Query {
			foo: String!
			bar: String!
		}
	`)
	sources := []*graphql.RemoteSchema{{Schema: schema, URL: "a"}}

	// the query we're going to fire should have two defined operations
	query := `
		query Foo {
			foo 
		}

		query Bar { 
			bar 
		}
	`

	// create a new schema with the sources and a planner that will respond with
	// values that have ids
	gateway, err := New(sources, WithPlanner(&MockPlanner{
		QueryPlanList{
			// the plan for the Foo operation
			&QueryPlan{
				FieldsToScrub: map[string][][]string{},
				Operation: &ast.OperationDefinition{
					Name:      "Foo",
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "foo",
							Definition: &ast.FieldDefinition{
								Type: ast.NamedType("String", &ast.Position{}),
							},
						},
					},
				},
				RootStep: &QueryPlanStep{
					Then: []*QueryPlanStep{
						{
							// this is equivalent to
							// query { allUsers }
							ParentType:     typeNameQuery,
							InsertionPoint: []string{},
							SelectionSet: ast.SelectionSet{
								&ast.Field{
									Name: "foo",
									Definition: &ast.FieldDefinition{
										Type: ast.NamedType("String", &ast.Position{}),
									},
								},
							},
							// return a known value we can test against
							Queryer: &graphql.MockSuccessQueryer{Value: map[string]interface{}{
								"foo": "foo",
							}},
						},
					},
				},
			},

			// the plan for the Bar operation
			&QueryPlan{
				FieldsToScrub: map[string][][]string{},
				Operation: &ast.OperationDefinition{
					Name:      "Bar",
					Operation: ast.Query,
					SelectionSet: ast.SelectionSet{
						&ast.Field{
							Name: "bar",
							Definition: &ast.FieldDefinition{
								Type: ast.NamedType("String", &ast.Position{}),
							},
						},
					},
				},
				RootStep: &QueryPlanStep{
					Then: []*QueryPlanStep{
						{
							// this is equivalent to
							// query { allUsers }
							ParentType:     typeNameQuery,
							InsertionPoint: []string{},
							SelectionSet: ast.SelectionSet{
								&ast.Field{
									Name: "bar",
									Definition: &ast.FieldDefinition{
										Type: ast.NamedType("String", &ast.Position{}),
									},
								},
							},
							// return a known value we can test against
							Queryer: &graphql.MockSuccessQueryer{Value: map[string]interface{}{
								"bar": "bar",
							}},
						},
					},
				},
			},
		},
	}))
	if err != nil {
		t.Error(err.Error())
		return
	}

	reqCtx := &RequestContext{
		Context: context.Background(), Query: query,
		OperationName: "Bar",
	}
	plan, err := gateway.GetPlans(reqCtx)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// execute the query
	res, err := gateway.Execute(reqCtx, plan)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure we didn't get any ids
	assert.Equal(t, map[string]interface{}{
		"bar": "bar",
	}, res)
}

func TestFieldURLs_concat(t *testing.T) {
	t.Parallel()
	// create a field url map
	first := FieldURLMap{}
	first.RegisterURL("Parent", "field1", "url1")
	first.RegisterURL("Parent", "field2", "url1")

	// create a second url map
	second := FieldURLMap{}
	second.RegisterURL("Parent", "field2", "url2")
	second.RegisterURL("Parent", "field3", "url2")

	// concatenate the 2
	sum := first.Concat(second)

	// make sure that that there is one entry for Parent.field1
	urlLocations1, err := sum.URLFor("Parent", "field1")
	if err != nil {
		t.Error(err.Error())
		return
	}
	assert.Equal(t, []string{"url1"}, urlLocations1)

	// look up the locations for Parent.field2
	urlLocations2, err := sum.URLFor("Parent", "field2")
	if err != nil {
		t.Error(err.Error())
		return
	}
	assert.Equal(t, []string{"url1", "url2"}, urlLocations2)

	// look up the locations for Parent.field3
	urlLocations3, err := sum.URLFor("Parent", "field3")
	if err != nil {
		t.Error(err.Error())
		return
	}
	assert.Equal(t, []string{"url2"}, urlLocations3)
}

// Verifies fix for https://github.com/nautilus/gateway/issues/199
func TestIncludeIfVariable(t *testing.T) {
	t.Parallel()
	schema, err := graphql.LoadSchema(`
type Query {
	hello: String
	user: User
}

type User {
	firstName: String
}
`)
	require.NoError(t, err)
	queryerFactory := QueryerFactory(func(ctx *PlanningContext, url string) graphql.Queryer {
		return graphql.QueryerFunc(func(input *graphql.QueryInput) (interface{}, error) {
			query := gqlparser.MustLoadQuery(schema, input.Query)
			var operation ast.OperationDefinition
			if assert.Len(t, query.Operations, 1) {
				operation = *query.Operations[0]
			}
			assert.Len(t, operation.VariableDefinitions, 1)
			assert.NotNil(t, operation.VariableDefinitions.ForName("returnUser"))
			assert.Equal(t, map[string]interface{}{
				"returnUser": true,
			}, input.Variables)
			return map[string]interface{}{
				"hello": "world",
				"user": map[string]interface{}{
					"firstName": "foo",
				},
			}, nil
		})
	})
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url1"},
	}, WithQueryerFactory(&queryerFactory))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`
		{
			"query": %q,
			"variables": {
				"returnUser": true
			}
		}
	`, `
		query($returnUser: Boolean!) {
			hello
			user @include(if: $returnUser) {
				firstName
			}
		}
	`)))
	resp := httptest.NewRecorder()
	gateway.GraphQLHandler(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
	assert.JSONEq(t, `
		{
			"data": {
				"hello": "world",
				"user": {
					"firstName": "foo"
				}
			}
		}
	`, resp.Body.String())
}

// TestDataAndErrorsBothReturnFromOneServicePartialSuccess verifies fix for https://github.com/nautilus/gateway/issues/212
func TestDataAndErrorsBothReturnFromOneServicePartialSuccess(t *testing.T) {
	t.Parallel()
	schema, err := graphql.LoadSchema(`
type Query {
	foo: String
	bar: String
}
`)
	require.NoError(t, err)
	queryerFactory := QueryerFactory(func(ctx *PlanningContext, url string) graphql.Queryer {
		return graphql.QueryerFunc(func(input *graphql.QueryInput) (interface{}, error) {
			return map[string]interface{}{
					"foo": "foo",
					"bar": nil,
				}, graphql.ErrorList{
					&graphql.Error{
						Message: "bar is broken",
						Path:    []interface{}{"bar"},
					},
				}
		})
	})
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "boo"},
	}, WithQueryerFactory(&queryerFactory))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"query": "query { foo bar }"}`))
	resp := httptest.NewRecorder()
	gateway.GraphQLHandler(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
	assert.JSONEq(t, `
		{
			"data": {
				"foo": "foo",
				"bar": null
			},
			"errors": [
				{
					"message": "bar is broken",
					"path": ["bar"],
					"extensions": null
				}
			]
		}
	`, resp.Body.String())
}

// TestGatewayRunsResponseMiddlewaresOnError verifies part of fix for https://github.com/nautilus/gateway/issues/212
// The issue included the 'id' field not getting scrubbed when an error was returned, and scrubbing is a builtin response middleware.
func TestGatewayRunsResponseMiddlewaresOnError(t *testing.T) {
	t.Parallel()
	schema, err := graphql.LoadSchema(`
type Query {
	foo: String
}
`)
	require.NoError(t, err)
	queryerFactory := QueryerFactory(func(ctx *PlanningContext, url string) graphql.Queryer {
		return graphql.QueryerFunc(func(input *graphql.QueryInput) (interface{}, error) {
			return map[string]interface{}{
					"foo": nil,
				}, graphql.ErrorList{
					&graphql.Error{
						Message: "foo is broken",
						Path:    []interface{}{"foo"},
					},
				}
		})
	})
	executedResponseMiddleware := false
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "boo"},
	}, WithQueryerFactory(&queryerFactory), WithMiddlewares(ResponseMiddleware(func(*ExecutionContext, map[string]interface{}) error {
		executedResponseMiddleware = true
		return nil
	})))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"query": "query { foo }"}`))
	resp := httptest.NewRecorder()
	gateway.GraphQLHandler(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
	assert.JSONEq(t, `
		{
			"data": {
				"foo": null
			},
			"errors": [
				{
					"message": "foo is broken",
					"path": ["foo"],
					"extensions": null
				}
			]
		}
	`, resp.Body.String())
	assert.True(t, executedResponseMiddleware, "All response middleware should run, even on responses with errors")
}

// TestPartialSuccessAlsoResolvesValidNodeIDs verifies fix for https://github.com/nautilus/gateway/issues/214
func TestPartialSuccessAlsoResolvesValidNodeIDs(t *testing.T) {
	t.Parallel()
	schemaFoo, err := graphql.LoadSchema(`
type Query {
	foo: Foo
}

type Foo {
	bar: Bar
	boo: String
}

interface Node {
	id: ID!
}

type Bar implements Node {
	id: ID!
}
`)
	require.NoError(t, err)
	schemaBar, err := graphql.LoadSchema(`
type Query {
	node(id: ID!): Node
}

interface Node {
	id: ID!
}

type Bar implements Node {
	id: ID!
	baz: String
}
`)
	require.NoError(t, err)
	const query = `
		query {
			foo {
				bar {
					baz
				}
			}
		}
	`
	queryerFactory := QueryerFactory(func(ctx *PlanningContext, url string) graphql.Queryer {
		return graphql.QueryerFunc(func(input *graphql.QueryInput) (interface{}, error) {
			t.Log("Received request:", input.Query)
			if strings.Contains(input.Query, "node(") {
				return map[string]interface{}{
					"node": map[string]interface{}{
						"baz": "biff",
					},
				}, nil
			}
			return map[string]interface{}{
					"foo": map[string]interface{}{
						"bar": map[string]interface{}{
							"id": "bar-id",
						},
						"boo": nil,
					},
				}, graphql.ErrorList{
					&graphql.Error{
						Message: "boo is broken",
						Path:    []interface{}{"foo", "boo"},
					},
				}
		})
	})
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schemaFoo, URL: "foo"},
		{Schema: schemaBar, URL: "bar"},
	}, WithQueryerFactory(&queryerFactory))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"query": %q}`, query)))
	resp := httptest.NewRecorder()
	gateway.GraphQLHandler(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
	assert.JSONEq(t, `
		{
			"data": {
				"foo": {
					"bar": {
						"baz": "biff"
					},
					"boo": null
				}
			},
			"errors": [
				{
					"message": "boo is broken",
					"path": ["foo", "boo"],
					"extensions": null
				}
			]
		}
	`, resp.Body.String())
}
