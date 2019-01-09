package graphql

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

func TestIntrospectQuery_savesQueryType(t *testing.T) {
	// introspect the api with a known response
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
	// introspect the api with a known response
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
	// introspect the api with a known response
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
	// introspect the api with a known response
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
	// introspect the api with a known response
	schema, err := IntrospectAPI(&MockQueryer{
		IntrospectionQueryResult{
			Schema: &IntrospectionQuerySchema{
				QueryType: IntrospectionQueryRootType{
					Name: "Query",
				},
				Types: []IntrospectionQueryFullType{
					{
						Kind:        "INTERFACE",
						Name:        "IFace",
						Description: "Description",
						Fields: []IntrospectionQueryFullTypeField{
							{
								Name: "Hello",
								Type: IntrospectionTypeRef{
									Kind: "SCALAR",
								},
							},
						},
					},
					{
						Kind: "OBJECT",
						Name: "Type1",
						Interfaces: []IntrospectionTypeRef{
							{
								Kind: "INTERFACE",
								Name: "IFace",
							},
						},
					},
					{
						Kind: "OBJECT",
						Name: "Type2",
						Interfaces: []IntrospectionTypeRef{
							{
								Kind: "INTERFACE",
								Name: "IFace",
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

	iface, ok := schema.Types["IFace"]
	if !ok {
		t.Error("Could not find union")
		return
	}

	// make sure the meta data was correct
	assert.Equal(t, "IFace", iface.Name)
	assert.Equal(t, "Description", iface.Description)

	// make sure there is only one field defined
	fields := iface.Fields
	if len(fields) != 1 {
		t.Errorf("Encountered incorrect number of fields on interface: %v", len(fields))
		return
	}
	assert.Equal(t, "Hello", fields[0].Name)

	// get the list of possible types that the implement the interface
	possibleTypes := schema.GetPossibleTypes(schema.Types["IFace"])
	if len(possibleTypes) != 2 {
		t.Errorf("Encountered incorrect number of fields that are possible for an interface: %v", len(possibleTypes))
		return
	}

	// make sure the first possible type matches expectations
	possibleType1 := possibleTypes[0]
	if possibleType1.Name != "Type1" && possibleType1.Name != "Type2" {
		t.Errorf("first possible type did not have the right name: %s", possibleType1.Name)
		return
	}

	// make sure the first possible type matches expectations
	possibleType2 := possibleTypes[0]
	if possibleType2.Name != "Type1" && possibleType2.Name != "Type2" {
		t.Errorf("first possible type did not have the right name: %s", possibleType2.Name)
		return
	}

	// make sure the 2 types implement the interface

	type1Implements := schema.GetImplements(schema.Types["Type1"])
	// type 1 implements only one type
	if len(type1Implements) != 1 {
		t.Errorf("Type1 implements incorrect number of types: %v", len(type1Implements))
		return
	}
	type1Implementer := type1Implements[0]
	assert.Equal(t, "IFace", type1Implementer.Name)

	type2Implements := schema.GetImplements(schema.Types["Type2"])
	// type 1 implements only one type
	if len(type2Implements) != 1 {
		t.Errorf("Type2 implements incorrect number of types: %v", len(type2Implements))
		return
	}
	type2Implementer := type2Implements[0]
	assert.Equal(t, "IFace", type2Implementer.Name)
}

func TestIntrospectQuery_unions(t *testing.T) {
	// introspect the api with a known response
	schema, err := IntrospectAPI(&MockQueryer{
		IntrospectionQueryResult{
			Schema: &IntrospectionQuerySchema{
				QueryType: IntrospectionQueryRootType{
					Name: "Query",
				},
				Types: []IntrospectionQueryFullType{
					{
						Kind:        "UNION",
						Name:        "Maybe",
						Description: "Description",
						PossibleTypes: []IntrospectionTypeRef{
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
	if err != nil {
		t.Error(err.Error())
		return
	}

	union, ok := schema.Types["Maybe"]
	if !ok {
		t.Error("Could not find union")
		return
	}

	// make sure the union matches epectations
	assert.Equal(t, "Maybe", union.Name)
	assert.Equal(t, ast.Union, union.Kind)
	assert.Equal(t, "Description", union.Description)

	// make sure that the possible types for the Union match expectations
	possibleTypes := schema.GetPossibleTypes(schema.Types["Maybe"])
	if len(possibleTypes) != 2 {
		t.Errorf("Encountered the right number of possible types: %v", len(possibleTypes))
		return
	}

	// make sure the first possible type matches expectations
	possibleType1 := possibleTypes[0]
	if possibleType1.Name != "Type1" && possibleType1.Name != "Type2" {
		t.Errorf("first possible type did not have the right name: %s", possibleType1.Name)
		return
	}

	// make sure the first possible type matches expectations
	possibleType2 := possibleTypes[0]
	if possibleType2.Name != "Type1" && possibleType2.Name != "Type2" {
		t.Errorf("first possible type did not have the right name: %s", possibleType2.Name)
		return
	}

	// make sure the 2 types implement the union

	type1Implements := schema.GetImplements(schema.Types["Type1"])
	// type 1 implements only one type
	if len(type1Implements) != 1 {
		t.Errorf("Type1 implements incorrect number of types: %v", len(type1Implements))
		return
	}
	type1Implementer := type1Implements[0]
	assert.Equal(t, "Maybe", type1Implementer.Name)

	type2Implements := schema.GetImplements(schema.Types["Type2"])
	// type 1 implements only one type
	if len(type2Implements) != 1 {
		t.Errorf("Type2 implements incorrect number of types: %v", len(type2Implements))
		return
	}
	type2Implementer := type2Implements[0]
	assert.Equal(t, "Maybe", type2Implementer.Name)
}

func TestIntrospectQueryUnmarshalType_scalarFields(t *testing.T) {
	// introspect the api with a known response
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
	// introspect the api with a known response
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
										Name:        "arg1",
										Description: "arg1-description",
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

func TestIntrospectQueryUnmarshalType_directives(t *testing.T) { // introspect the api with a known response
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
										Name:        "arg1",
										Description: "arg1-description",
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
				Directives: []IntrospectionQueryDirective{
					{
						Name:        "internal",
						Description: "internal-description",
						Locations:   []string{"QUERY", "MUTATION"},
						Args: []IntrospectionInputValue{
							{
								Name:        "hello",
								Description: "hello-description",
								Type: IntrospectionTypeRef{
									Name: "String",
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

	// make sure we have the one definition
	if len(schema.Directives) != 1 {
		t.Errorf("Encountered incorrect number of directives: %v", len(schema.Types))
		return
	}

	directive, ok := schema.Directives["internal"]
	if !ok {
		t.Error("Could not find directive 'internal'")
		return
	}

	// make sure that the directive meta data is right
	assert.Equal(t, "internal", directive.Name)
	assert.Equal(t, "internal-description", directive.Description)
	assert.Equal(t, []ast.DirectiveLocation{ast.LocationQuery, ast.LocationMutation}, directive.Locations)

	// make sure we got the args right
	if len(directive.Arguments) != 1 {
		t.Errorf("Encountered incorrect number of arguments: %v", len(directive.Arguments))
		return
	}

	// make sure we got the argumen type right
	assert.Equal(t, "String", directive.Arguments[0].Type.Name())
}

func TestIntrospectQueryUnmarshalType_enums(t *testing.T) {
	// introspect the api with a known response
	schema, err := IntrospectAPI(&MockQueryer{
		IntrospectionQueryResult{
			Schema: &IntrospectionQuerySchema{
				QueryType: IntrospectionQueryRootType{
					Name: "Query",
				},
				Types: []IntrospectionQueryFullType{
					IntrospectionQueryFullType{
						Kind:        "ENUM",
						Name:        "Word",
						Description: "enum-description",
						EnumValues: []IntrospectionQueryEnumDefinition{
							{
								Name:        "hello",
								Description: "hello-description",
							},
							{
								Name:        "goodbye",
								Description: "goodbye-description",
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

	// make sure we have the one definitino
	if len(schema.Types) != 1 {
		t.Errorf("Encountered incorrect number of types: %v", len(schema.Types))
		return
	}

	enum, ok := schema.Types["Word"]
	if !ok {
		t.Error("Coud not find definition for Word enum")
		return
	}

	// make sure the values matched expectations
	assert.Equal(t, "Word", enum.Name)
	assert.Equal(t, ast.Enum, enum.Kind)
	assert.Equal(t, "enum-description", enum.Description)
	assert.Equal(t, enum.EnumValues, ast.EnumValueList{
		&ast.EnumValueDefinition{
			Name:        "hello",
			Description: "hello-description",
		},
		&ast.EnumValueDefinition{
			Name:        "goodbye",
			Description: "goodbye-description",
		},
	})
}

func TestIntrospectQuery_deprecatedFields(t *testing.T) {
	t.Skip("Not yet implemented")
}

func TestIntrospectQuery_deprecatedEnums(t *testing.T) {
	t.Skip("Not yet Implemented")
}

func TestIntrospectQueryUnmarshalType_inputObjects(t *testing.T) {
	// introspect the api with a known response
	schema, err := IntrospectAPI(&MockQueryer{
		IntrospectionQueryResult{
			Schema: &IntrospectionQuerySchema{
				QueryType: IntrospectionQueryRootType{
					Name: "Query",
				},
				Types: []IntrospectionQueryFullType{
					IntrospectionQueryFullType{
						Kind:        "INPUT_OBJECT",
						Name:        "InputObjectType",
						Description: "Description",
						InputFields: []IntrospectionInputValue{
							{
								Name:        "hello",
								Description: "hello-description",
								Type: IntrospectionTypeRef{
									Name: "String",
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
	object, ok := schema.Types["InputObjectType"]
	if !ok {
		t.Error("Could not find a reference to Query object")
		return
	}

	// make sure the object meta data is right
	assert.Equal(t, "InputObjectType", object.Name)
	assert.Equal(t, "Description", object.Description)

	// we should have added a single field
	if len(object.Fields) != 1 {
		t.Errorf("Encountered incorrect number of fields: %v", len(object.Fields))
		return
	}
	field := object.Fields[0]

	// make sure it had the right metadata
	assert.Equal(t, "hello", field.Name)
	assert.Equal(t, "hello-description", field.Description)
	assert.Equal(t, "String", field.Type.Name())
}

func TestIntrospectUnmarshalDirectiveLocation(t *testing.T) {
	// make sure each directive location is extractable
	for key, value := range directiveLocationMap {
		// make sure we can convert a list of strings to the list of location
		result, err := introspectionUnmarshalDirectiveLocation([]string{key, key})
		if err != nil {
			t.Error(err.Error())
			return
		}

		assert.Equal(t, []ast.DirectiveLocation{value, value}, result)
	}
}

func TestIntrospectUnmarshalTypeDef(t *testing.T) {
	// the table
	table := []struct {
		Message    string
		Expected   *ast.Type
		RemoteType *IntrospectionTypeRef
	}{
		// named types
		{
			"User",
			ast.NamedType("User", &ast.Position{}),
			&IntrospectionTypeRef{
				Kind: "OBJECT",
				Name: "User",
			},
		},
		// non-null named types
		{
			"User!",
			ast.NonNullNamedType("User", &ast.Position{}),
			&IntrospectionTypeRef{
				Kind: "NON_NULL",
				OfType: &IntrospectionTypeRef{
					Kind: "OBJECT",
					Name: "User",
				},
			},
		},
		// lists of named types
		{
			"[User]",
			ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
			&IntrospectionTypeRef{
				Kind: "LIST",
				OfType: &IntrospectionTypeRef{
					Kind: "OBJECT",
					Name: "User",
				},
			},
		},
		// non-null list of named types
		{
			"[User]!",
			ast.NonNullListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}),
			&IntrospectionTypeRef{
				Kind: "NON_NULL",
				OfType: &IntrospectionTypeRef{
					Kind: "LIST",
					OfType: &IntrospectionTypeRef{
						Kind: "OBJECT",
						Name: "User",
					},
				},
			},
		},
		// a non-null list of non-null types
		{
			"[User!]!",
			ast.NonNullListType(ast.NonNullNamedType("User", &ast.Position{}), &ast.Position{}),
			&IntrospectionTypeRef{
				Kind: "NON_NULL",
				OfType: &IntrospectionTypeRef{
					Kind: "LIST",
					OfType: &IntrospectionTypeRef{
						Kind: "NON_NULL",
						OfType: &IntrospectionTypeRef{
							Kind: "OBJECT",
							Name: "User",
						},
					},
				},
			},
		},
		// lists of lists of named types
		{
			"[[User]]",
			ast.ListType(ast.ListType(ast.NamedType("User", &ast.Position{}), &ast.Position{}), &ast.Position{}),
			&IntrospectionTypeRef{
				Kind: "LIST",
				OfType: &IntrospectionTypeRef{
					Kind: "LIST",
					OfType: &IntrospectionTypeRef{
						Kind: "OBJECT",
						Name: "User",
					},
				},
			},
		},
	}

	for _, row := range table {
		assert.Equal(t, row.Expected, introspectionUnmarshalTypeRef(row.RemoteType), fmt.Sprintf("Desired type: %s", row.Message))
	}
}
