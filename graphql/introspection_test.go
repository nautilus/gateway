package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntrospectQuery_savesQueryType(t *testing.T) {
	// introspect tIhe api with a known response
	schema, err := IntrospectAPI(&MockQueryer{
		IntrospectionQueryResult{
			Schema: &IntrospectionQuerySchema{
				QueryType: IntrospectionQueryRootType{
					Name: "Query",
				},
				Types: []IntrospectionQueryFullType{
					{
						Kind: "OBJECT",
						Name: "Query",
						Fields: []IntrospectionQueryFullTypeField{
							{
								Name: "Hello",
								Type: IntrospectionTypeRef{
									Kind: "SCALAR",
								},
							},
						},
					},
				},
			},
		},
	})
	// if something went wrong
	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure we got a schema back
	if schema == nil {
		t.Error("Received nil schema")
		return
	}
	if schema.Query == nil {
		t.Error("Query was nil")
		return
	}

	// make sure the query type has the right name
	assert.Equal(t, "Query", schema.Query.Name)
}

func TestIntrospectQuery_savesMutationType(t *testing.T) {
	// introspect tIhe api with a known response
	schema, err := IntrospectAPI(&MockQueryer{
		IntrospectionQueryResult{
			Schema: &IntrospectionQuerySchema{
				QueryType: IntrospectionQueryRootType{
					Name: "Query",
				},
				MutationType: &IntrospectionQueryRootType{
					Name: "Mutation",
				},
				Types: []IntrospectionQueryFullType{
					{
						Kind: "OBJECT",
						Name: "Mutation",
						Fields: []IntrospectionQueryFullTypeField{
							{
								Name: "Hello",
								Type: IntrospectionTypeRef{
									Kind: "SCALAR",
								},
							},
						},
					},
				},
			},
		},
	})
	// if something went wrong
	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure we got a schema back
	if schema == nil {
		t.Error("Received nil schema")
		return
	}
	if schema.Mutation == nil {
		t.Error("Mutation was nil")
		return
	}

	// make sure the query type has the right name
	assert.Equal(t, "Mutation", schema.Mutation.Name)
}

func TestIntrospectQuery_savesSubscriptionType(t *testing.T) {
	// introspect tIhe api with a known response
	schema, err := IntrospectAPI(&MockQueryer{
		IntrospectionQueryResult{
			Schema: &IntrospectionQuerySchema{
				QueryType: IntrospectionQueryRootType{
					Name: "Query",
				},
				SubscriptionType: &IntrospectionQueryRootType{
					Name: "Subscription",
				},
				Types: []IntrospectionQueryFullType{
					{
						Kind: "OBJECT",
						Name: "Subscription",
						Fields: []IntrospectionQueryFullTypeField{
							{
								Name: "Hello",
								Type: IntrospectionTypeRef{
									Kind: "SCALAR",
								},
							},
						},
					},
				},
			},
		},
	})
	// if something went wrong
	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure we got a schema back
	if schema == nil {
		t.Error("Received nil schema")
		return
	}
	if schema.Subscription == nil {
		t.Error("Subscription was nil")
		return
	}

	// make sure the query type has the right name
	assert.Equal(t, "Subscription", schema.Subscription.Name)
}
