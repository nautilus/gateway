package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
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

func TestIntrospectQuery_multipleTypes(t *testing.T) {
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
						Name: "Type1",
					},
					{
						Kind: "OBJECT",
						Name: "Type2",
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

	// make sure that the schema has both types
	if len(schema.Types) != 2 {
		t.Errorf("Encounted incorrect number of types: %v", len(schema.Types))
		return
	}

	// there should be Type1
	type1, ok := schema.Types["Type1"]
	if !ok {
		t.Errorf("Did not have a type 1")
		return
	}
	assert.Equal(t, "Type1", type1.Name)
	assert.Equal(t, ast.Object, type1.Kind)

	// there should be Type2
	type2, ok := schema.Types["Type2"]
	if !ok {
		t.Errorf("Did not have a type 2")
		return
	}
	assert.Equal(t, "Type2", type2.Name)
	assert.Equal(t, ast.Object, type2.Kind)
}

func TestIntrospectQuery_interfaces(t *testing.T) {
	t.Skip("Not yet implemented.")
}

func TestIntrospectQuery_union(t *testing.T) {
	t.Skip("Not yet implemented.")
}

func TestIntrospectQueryUnmarshalType_scalarFields(t *testing.T) {
	// introspect tIhe api with a known response
	schema, err := IntrospectAPI(&MockQueryer{
		IntrospectionQueryResult{
			Schema: &IntrospectionQuerySchema{
				QueryType: IntrospectionQueryRootType{
					Name: "Query",
				},
				Types: []IntrospectionQueryFullType{
					IntrospectionQueryFullType{
						Kind:        "SCALAR",
						Name:        "Name",
						Description: "Description",
					},
				},
			},
		},
	})
	if err != nil {
		t.Error(err.Error())
		return
	}

	// create a scalar type with known characteristics
	scalar, ok := schema.Types["Name"]
	if !ok {
		t.Error("Could not find a reference to Name scalar")
		return
	}

	// make sure the scalar has the right meta data
	assert.Equal(t, ast.Scalar, scalar.Kind)
	assert.Equal(t, "Name", scalar.Name)
	assert.Equal(t, "Description", scalar.Description)
}

func TestIntrospectQueryUnmarshalType_objects(t *testing.T) {
	// introspect tIhe api with a known response
	schema, err := IntrospectAPI(&MockQueryer{
		IntrospectionQueryResult{
			Schema: &IntrospectionQuerySchema{
				QueryType: IntrospectionQueryRootType{
					Name: "Query",
				},
				Types: []IntrospectionQueryFullType{
					IntrospectionQueryFullType{
						Kind:        "OBJECT",
						Name:        "Query",
						Description: "Description",
						Fields: []IntrospectionQueryFullTypeField{
							{
								Name:        "hello",
								Description: "field-description",
								Args: []IntrospectionInputValue{
									{
										Name:         "arg1",
										Description:  "arg1-description",
										DefaultValue: "arg1-default-value",
										Type: IntrospectionTypeRef{
											Name: "String",
										},
									},
								},
								Type: IntrospectionTypeRef{
									Name: "Foo",
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Error(err.Error())
		return
	}

	// create a scalar type with known characteristics
	object, ok := schema.Types["Query"]
	if !ok {
		t.Error("Could not find a reference to Query object")
		return
	}

	// make sure the object has the right meta data
	assert.Equal(t, ast.Object, object.Kind)
	assert.Equal(t, "Query", object.Name)
	assert.Equal(t, "Description", object.Description)

	// we should have added a single field
	if len(object.Fields) != 1 {
		t.Errorf("Encountered incorrect number of fields: %v", len(object.Fields))
		return
	}
	field := object.Fields[0]

	// make sure it had the right metadata
	assert.Equal(t, "hello", field.Name)
	assert.Equal(t, "field-description", field.Description)
	assert.Equal(t, "Foo", field.Type.Name())

	// it should have one arg
	if len(field.Arguments) != 1 {
		t.Errorf("Encountered incorrect number of arguments: %v", len(field.Arguments))
		return
	}
	argument := field.Arguments[0]

	// make sure it has the right metadata
	assert.Equal(t, "arg1", argument.Name)
	assert.Equal(t, "arg1-description", argument.Description)
	assert.Equal(t, "String", argument.Type.Name())
}

func TestIntrospectQueryUnmarshalType_directives(t *testing.T) {
	t.Skip("Not yet implemented.")
}

func TestIntrospectQueryUnmarshalType_enums(t *testing.T) {
	t.Skip("Not yet implemented.")
}

func TestIntrospectQueryUnmarshalType_inputObjects(t *testing.T) {
	t.Skip("Not yet implemented.")
}
