package gateway

import (
	"testing"

	"github.com/alecaivazis/graphql-gateway/graphql"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
)

func schemaTestLoadQuery(query string, target interface{}) error {
	schema, _ := graphql.LoadSchema(`
		type User {
			firstName: String!
		}

		type Query {
			"description"
			allUsers: [User]
		}
	`)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url1"},
	})
	if err != nil {
		return err
	}

	// executing the introspection query should return a full description of the schema
	response, err := gateway.Execute(query)
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

func TestSchemaIntrospection(t *testing.T) {
	// a place to hold the response of the query
	result := &graphql.IntrospectionQueryResult{}

	// a place to hold the response of the query
	err := schemaTestLoadQuery(graphql.IntrospectionQuery, result)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// there are a few things we need to look for:
	// 		Schema.queryType.name, Schema.mutationType, Schema.subscriptionType, Query.allUsers, and User.firstName
	assert.Equal(t, "Query", result.Schema.QueryType.Name)
	assert.Nil(t, result.Schema.MutationType)
	assert.Nil(t, result.Schema.SubscriptionType)

	// definitions for the types we want to investigate
	var queryType graphql.IntrospectionQueryFullType
	var userType graphql.IntrospectionQueryFullType
	for _, schemaType := range result.Schema.Types {
		if schemaType.Name == "Query" {
			queryType = schemaType
		} else if schemaType.Name == "User" {
			userType = schemaType
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
}

func TestSchemaIntrospection_typeNotFound(*testing.T) {

}
