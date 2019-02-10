package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nautilus/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

type schemaTableRow struct {
	location string
	query    string
}

func TestGateway(t *testing.T) {
	schemas := []schemaTableRow{
		{
			"url1",
			`
				type Query {
					allUsers: [User!]!
				}

				type User {
					id: ID!
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
		locations := fieldURLs(sources, false)

		allUsersURL, err := locations.URLFor("Query", "allUsers")
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

	t.Run("Variadic Configuration", func(t *testing.T) {
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

	t.Run("fieldURLs ignore introspection", func(t *testing.T) {
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

	t.Run("Configurator WithPlanner", func(t *testing.T) {
		// the planner we will assign
		planner := &MockPlanner{}

		gateway, err := New(sources, WithPlanner(planner))
		if err != nil {
			t.Error(err.Error())
			return
		}

		assert.Equal(t, planner, gateway.planner)
	})

	t.Run("Response Middleware Error", func(t *testing.T) {
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
		_, err = gateway.Execute(context.Background(), `{ allUsers { firstName } }`, map[string]interface{}{})
		if err == nil {
			t.Errorf("Did not encounter error executing plan.")
		}
	})

	t.Run("Response Middleware Success", func(t *testing.T) {
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

		// build a query plan that the executor will follow
		response, err := gateway.Execute(context.Background(), `{ allUsers { firstName } }`, map[string]interface{}{})

		if err != nil {
			t.Errorf("Encountered error executing plan: %s", err.Error())
			return
		}
		// make sure our middleware changed the response
		assert.Equal(t, map[string]interface{}{"hello": "world"}, response)
	})

	t.Run("filter out automatically inserted ids", func(t *testing.T) {
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
			Plans: []*QueryPlan{
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
								ParentType:     "Query",
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
								Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
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
										Queryer: &graphql.MockSuccessQueryer{map[string]interface{}{
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

		// execute the query
		res, err := gateway.Execute(context.Background(), query, map[string]interface{}{})
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

	t.Run("Gateway fields", func(t *testing.T) {
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

		// execute the query
		query := `
			{
				viewer(id: "1") {
					id
				}
			}
		`
		res, err := gateway.Execute(context.Background(), query, map[string]interface{}{"id": "1"})
		if err != nil {
			t.Error(err.Error())
			return
		}

		// make sure we got the result we expected
		assert.Equal(t, map[string]interface{}{"id": "1"}, res)
	})
}

func TestFieldURLs_concat(t *testing.T) {
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
