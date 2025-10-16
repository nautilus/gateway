package gateway

import (
	"context"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/nautilus/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func schemaTestLoadQuery(query string, target interface{}, variables map[string]interface{}) error {
	schema, _ := graphql.LoadSchema(`
		type User {
			firstName: String!
		}

		type Query {
			"description"
			allUsers: [User]
		}

		enum EnumValue {
			"foo description"
			FOO
			BAR
		}

		input FooInput {
			foo: String
		}

		directive @A on FIELD_DEFINITION
	`)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url1"},
	})
	if err != nil {
		return err
	}

	reqCtx := &RequestContext{
		Context:   context.Background(),
		Query:     query,
		Variables: variables,
	}
	plan, err := gateway.GetPlans(reqCtx)
	if err != nil {
		return err
	}

	// executing the introspection query should return a full description of the schema
	response, err := gateway.Execute(reqCtx, plan)
	if err != nil {
		return err
	}

	// massage the map into the structure
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "json",
		Result:  target,
	})
	if err != nil {
		return err
	}
	err = decoder.Decode(response)
	if err != nil {
		return err
	}

	return nil
}

func TestSchemaIntrospection_query(t *testing.T) {
	t.Parallel()
	// a place to hold the response of the query
	result := &graphql.IntrospectionQueryResult{}

	// a place to hold the response of the query
	err := schemaTestLoadQuery(graphql.IntrospectionQuery, result, map[string]interface{}{})
	if err != nil {
		t.Error(err.Error())
		return
	}

	// there are a few things we need to look for:
	// 		Schema.queryType.name, Schema.mutationType, Schema.subscriptionType, Query.allUsers, and User.firstName
	assert.Equal(t, typeNameQuery, result.Schema.QueryType.Name)
	assert.Nil(t, result.Schema.MutationType)
	assert.Nil(t, result.Schema.SubscriptionType)

	// definitions for the types we want to investigate
	var queryType graphql.IntrospectionQueryFullType
	var userType graphql.IntrospectionQueryFullType
	var enumType graphql.IntrospectionQueryFullType
	var fooInput graphql.IntrospectionQueryFullType

	for _, schemaType := range result.Schema.Types {
		switch schemaType.Name {
		case typeNameQuery:
			queryType = schemaType
		case "User":
			userType = schemaType
		case "EnumValue":
			enumType = schemaType
		case "FooInput":
			fooInput = schemaType
		}
	}

	// make sure that Query.allUsers looks as expected
	var allUsersField graphql.IntrospectionQueryFullTypeField
	for _, field := range queryType.Fields {
		if field.Name == "allUsers" {
			allUsersField = field
		}
	}

	// make sure the type definition for the field matches expectation
	assert.Equal(t, graphql.IntrospectionTypeRef{
		Kind: "LIST",
		OfType: &graphql.IntrospectionTypeRef{
			Kind: "OBJECT",
			Name: "User",
		},
	}, allUsersField.Type)
	assert.Equal(t, "description", allUsersField.Description)

	// make sure that Query.allUsers looks as expected
	var firstNameField graphql.IntrospectionQueryFullTypeField
	for _, field := range userType.Fields {
		if field.Name == "firstName" {
			firstNameField = field
		}
	}

	// make sure the type definition for the field matches expectation
	assert.Equal(t, graphql.IntrospectionTypeRef{
		Kind: "NON_NULL",
		OfType: &graphql.IntrospectionTypeRef{
			Kind: "SCALAR",
			Name: "String",
		},
	}, firstNameField.Type)

	// make sure that the enums have the right values
	assert.Equal(t, "EnumValue", enumType.Name)
	assert.Equal(t, []graphql.IntrospectionQueryEnumDefinition{
		{
			Name:              "FOO",
			Description:       "foo description",
			IsDeprecated:      false,
			DeprecationReason: "",
		},
		{
			Name:              "BAR",
			Description:       "",
			IsDeprecated:      false,
			DeprecationReason: "",
		},
	}, enumType.EnumValues)

	// make sure the foo input matches exepectations
	assert.Equal(t, "FooInput", fooInput.Name)
	assert.Equal(t, []graphql.IntrospectionInputValue{
		{
			Name: "foo",
			Type: graphql.IntrospectionTypeRef{
				Kind: "SCALAR",
				Name: "String",
			},
		},
	}, fooInput.InputFields)

	// grab the directive we've defined
	var directive graphql.IntrospectionQueryDirective
	for _, definition := range result.Schema.Directives {
		if definition.Name == "A" {
			directive = definition
		}
	}
	assert.Equal(t, "A", directive.Name)
}

func TestSchemaIntrospection_deterministicOrder(t *testing.T) {
	t.Parallel()
	introspectSchema := func() graphql.IntrospectionQueryResult {
		var result graphql.IntrospectionQueryResult
		err := schemaTestLoadQuery(graphql.IntrospectionQuery, &result, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Log(result)
		return result
	}

	first := introspectSchema()
	const maxAttempts = 10
	for i := 0; i < maxAttempts; i++ {
		require.Equal(t, first, introspectSchema())
	}
}

func TestSchemaIntrospection_lookUpType(t *testing.T) {
	t.Parallel()
	// a place to hold the response of the query
	result := &struct {
		Type struct {
			Name string `json:"name"`
		} `json:"__type"`
	}{}

	query := `
		{
			__type(name: "User") {
				name
			}
		}
	`

	// a place to hold the response of the query
	err := schemaTestLoadQuery(query, result, map[string]interface{}{})
	if err != nil {
		t.Error(err.Error())
		return
	}

	assert.Equal(t, "User", result.Type.Name)
}

func TestSchemaIntrospection_missingType(t *testing.T) {
	t.Parallel()
	// a place to hold the response of the query
	result := &struct {
		Type *struct {
			Name string `json:"name"`
		} `json:"__type"`
	}{}

	query := `
		{
			__type(name: "Foo") {
				name
			}
		}
	`

	// a place to hold the response of the query
	err := schemaTestLoadQuery(query, result, map[string]interface{}{})
	if err != nil {
		t.Error(err.Error())
		return
	}

	assert.Nil(t, result.Type)
}

func TestSchemaIntrospection_missingQueryDocument(t *testing.T) {
	t.Parallel()
	schema, err := graphql.LoadSchema("")
	require.NoError(t, err)
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url1"},
	})
	require.NoError(t, err)

	t.Run("valid query", func(t *testing.T) {
		t.Parallel()
		var result interface{}
		err := gateway.Query(context.Background(), &graphql.QueryInput{
			Query: `
			{
				__type(name: "Query") {
					name
				}
			}
		`,
			QueryDocument: nil,
		}, &result)
		assert.NoError(t, err)
		name := "Query"
		assert.Equal(t, map[string]interface{}{
			"__type": map[string]interface{}{
				"name": &name,
			},
		}, result)
	})

	t.Run("invalid query", func(t *testing.T) {
		t.Parallel()
		var result interface{}
		err := gateway.Query(context.Background(), &graphql.QueryInput{
			Query:         "garbage",
			QueryDocument: nil,
		}, &result)
		assert.EqualError(t, err, "input:1: Unexpected Name \"garbage\"\n")
	})
}

func TestSchema_resolveNodeInlineID(t *testing.T) {
	t.Parallel()
	type Result struct {
		Node struct {
			ID string `json:"id"`
		} `json:"node"`
	}

	// a place to hold the response of the query
	result := &Result{}

	query := `
		{
			node(id: "my-id") {
				id
			}
		}
	`

	// a place to hold the response of the query
	err := schemaTestLoadQuery(query, result, map[string]interface{}{})
	if err != nil {
		t.Error(err.Error())
		return
	}

	assert.Equal(t, &Result{Node: struct {
		ID string `json:"id"`
	}{ID: "my-id"}}, result)
}

func TestSchema_resolveNodeIDFromArg(t *testing.T) {
	t.Parallel()
	type Result struct {
		Node struct {
			ID string `json:"id"`
		} `json:"node"`
	}

	// a place to hold the response of the query
	result := &Result{}

	query := `
		query($id: ID!){
			node(id: $id) {
				id
			}
		}
	`

	// a place to hold the response of the query
	err := schemaTestLoadQuery(query, result, map[string]interface{}{
		"id": "my-id",
	})
	if err != nil {
		t.Error(err.Error())
		return
	}

	assert.Equal(t, &Result{Node: struct {
		ID string `json:"id"`
	}{ID: "my-id"}}, result)
}

func TestSchema_resolveNodeWrongIDType(t *testing.T) {
	t.Parallel()
	type Node struct {
		ID *string `json:"id"`
	}
	type Result struct {
		Node Node `json:"node"`
	}
	var result Result

	query := `
		{
			node(id: 123) {
				id
			}
		}
	`

	err := schemaTestLoadQuery(query, result, map[string]interface{}{})
	assert.Equal(t, graphql.ErrorList{
		&graphql.Error{
			Message: "invalid ID type: 123",
			Path:    []interface{}{"node"},
		},
	}, err)
	assert.Equal(t, Result{
		Node: Node{ID: nil},
	}, result)
}

func TestSchema_resolveNodeMissingIDArg(t *testing.T) {
	t.Parallel()
	type Node struct {
		ID *string `json:"id"`
	}
	type Result struct {
		Node Node `json:"node"`
	}
	var result Result

	query := `
		query ($id: ID!) {
			node(id: $id) {
				id
			}
		}
	`
	variables := map[string]interface{}{} // missing ID arg

	err := schemaTestLoadQuery(query, result, variables)
	assert.Equal(t, graphql.ErrorList{
		&graphql.Error{
			Message: "argument 'id' is required",
			Path:    []interface{}{"node"},
		},
	}, err)
	assert.Equal(t, Result{
		Node: Node{ID: nil},
	}, result)
}

func TestSchema_resolveNodeWrongIDArgType(t *testing.T) {
	t.Parallel()
	type Node struct {
		ID *string `json:"id"`
	}
	type Result struct {
		Node Node `json:"node"`
	}
	var result Result

	query := `
		query ($id: ID!) {
			node(id: $id) {
				id
			}
		}
	`
	variables := map[string]interface{}{
		"id": 123,
	}

	err := schemaTestLoadQuery(query, result, variables)
	assert.Equal(t, graphql.ErrorList{
		&graphql.Error{
			Message: "invalid ID type: 123",
			Path:    []interface{}{"node"},
		},
	}, err)
	assert.Equal(t, Result{
		Node: Node{ID: nil},
	}, result)
}

func TestSchema_resolveNodeWrongIDTypeWithAlias(t *testing.T) {
	t.Parallel()
	type Node struct {
		ID *string `json:"id"`
	}
	type Result struct {
		Node Node `json:"node"`
	}
	var result Result

	query := `
		{
			myAlias: node(id: 123) {
				id
			}
		}
	`

	err := schemaTestLoadQuery(query, result, map[string]interface{}{})
	assert.Equal(t, graphql.ErrorList{
		&graphql.Error{
			Message: "invalid ID type: 123",
			Path:    []interface{}{"myAlias"},
		},
	}, err)
	assert.Equal(t, Result{
		Node: Node{ID: nil},
	}, result)
}
