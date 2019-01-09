package graphql

import (
	"errors"

	"github.com/vektah/gqlparser/ast"
)

// IntrospectAPI send the introspection query to a Queryer and builds up the
// schema object described by the result
func IntrospectAPI(queryer Queryer) (*ast.Schema, error) {
	// a place to hold the result of firing the introspection query
	result := IntrospectionQueryResult{}

	// fire the introspection query
	err := queryer.Query(&QueryInput{Query: IntrospectionQuery}, &result)
	if err != nil {
		return nil, err
	}

	// grab the schema
	remoteSchema := result.Schema

	// create a schema we will build up over time
	schema := &ast.Schema{
		Types: map[string]*ast.Definition{},
	}

	// if we dont have a name on the response
	if remoteSchema.QueryType.Name == "" {
		return nil, errors.New("Could not find the root query")
	}

	// reconstructing the schema happens in a few pass throughs
	// the first builds a map of type names to their definition
	// the second pass goes over the definitions and reconstructs the types

	// add each type to the schema
	for _, remoteType := range remoteSchema.Types {
		// convert turn the API payload into a schema type
		schemaType := introspectionUnmarshalType(remoteType)

		// check if this type is the QueryType
		if remoteType.Name == remoteSchema.QueryType.Name {
			schema.Query = schemaType
		} else if remoteSchema.MutationType != nil && schemaType.Name == remoteSchema.MutationType.Name {
			schema.Mutation = schemaType
		} else if remoteSchema.SubscriptionType != nil && schemaType.Name == remoteSchema.SubscriptionType.Name {
			schema.Subscription = schemaType
		}

		// register the type with the schema
		schema.Types[schemaType.Name] = schemaType
	}

	// the second pass constructs the fields and
	for _, remoteType := range remoteSchema.Types {
		// a reference to the type
		storedType, ok := schema.Types[remoteType.Name]
		if !ok {
			return nil, err
		}

		// build up a list of fields associated with the type
		fields := ast.FieldList{}
		for _, field := range remoteType.Fields {
			// build up the field for this one
			schemaField := &ast.FieldDefinition{
				Name:        field.Name,
				Type:        introspectionUnmarshalTypeRef(&field.Type),
				Description: field.Description,
				Arguments:   introspectionConvertArgList(field.Args),
			}

			// add the field to the list
			fields = append(fields, schemaField)
		}

		// save the list of fields in the schema type
		storedType.Fields = fields
	}

	// we're done here
	return schema, nil
}

func introspectionConvertArgList(args []IntrospectionInputValue) ast.ArgumentDefinitionList {
	result := ast.ArgumentDefinitionList{}

	// we need to add each argument to the field
	for _, argument := range args {
		result = append(result, &ast.ArgumentDefinition{
			Name:        argument.Name,
			Description: argument.Description,
			Type:        introspectionUnmarshalTypeRef(&argument.Type),
		})
	}

	return result
}

func introspectionUnmarshalType(schemaType IntrospectionQueryFullType) *ast.Definition {
	definition := &ast.Definition{
		Name:        schemaType.Name,
		Description: schemaType.Description,
	}

	// the kind of type
	switch schemaType.Kind {
	case "OBJECT":
		definition.Kind = ast.Object
	case "SCALAR":
		definition.Kind = ast.Scalar
	case "INTERFACE":
		definition.Kind = ast.Interface
	case "UNION":
		definition.Kind = ast.Union
	case "ENUM":
		definition.Kind = ast.Enum
		// save the enum values
		definition.EnumValues = ast.EnumValueList{}

		// convert each enum value into the appropriate object
		for _, value := range schemaType.EnumValues {
			definition.EnumValues = append(definition.EnumValues, &ast.EnumValueDefinition{
				Name:        value.Name,
				Description: value.Description,
			})

		}

	}

	return definition
}

func introspectionUnmarshalTypeRef(response *IntrospectionTypeRef) *ast.Type {
	// we could have a non-null list of a field
	if response.Kind == "NON_NULL" && response.OfType.Kind == "LIST" {
		return ast.NonNullListType(introspectionUnmarshalTypeRef(response.OfType.OfType), &ast.Position{})
	}

	// we could have a list of a type
	if response.Kind == "LIST" {
		return ast.ListType(introspectionUnmarshalTypeRef(response.OfType), &ast.Position{})
	}

	// we could have just a non null
	if response.Kind == "NON_NULL" {
		return ast.NonNullNamedType(response.OfType.Name, &ast.Position{})
	}

	// if we are looking at a named type that isn't in a list or marked non-null
	return ast.NamedType(response.Name, &ast.Position{})
}

type IntrospectionQueryResult struct {
	Schema *IntrospectionQuerySchema `json:"__schema"`
}

type IntrospectionQuerySchema struct {
	QueryType        IntrospectionQueryRootType   `json:"queryType"`
	MutationType     *IntrospectionQueryRootType  `json:"mutationType"`
	SubscriptionType *IntrospectionQueryRootType  `json:"subscriptionType"`
	Types            []IntrospectionQueryFullType `json:"types"`
	Directives       []struct {
		Name        string                    `json:"name"`
		Description string                    `json:"description"`
		Locations   []string                  `json:"location"`
		Args        []IntrospectionInputValue `json:"arg"`
	}
}

type IntrospectionQueryRootType struct {
	Name string `json:"name"`
}

type IntrospectionQueryFullTypeField struct {
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	Args              []IntrospectionInputValue `json:"args"`
	Type              IntrospectionTypeRef      `json:"type"`
	IsDeprecated      bool                      `json:"isDeprecated"`
	DeprecationReason string                    `json:"deprecationReason"`
}

type IntrospectionQueryFullType struct {
	Kind          string                             `json:"kind"`
	Name          string                             `json:"name"`
	Description   string                             `json:"description"`
	InputFields   []IntrospectionInputValue          `json:"inputField"`
	Interfaces    []IntrospectionTypeRef             `json:"interfaces"`
	PossibleTypes []IntrospectionTypeRef             `json:"possibleTypes"`
	Fields        []IntrospectionQueryFullTypeField  `json:"fields"`
	EnumValues    []IntrospectionQueryEnumDefinition `json:"enumValues"`
}

type IntrospectionQueryEnumDefinition struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	IsDeprecated      bool   `json:"isDeprecated"`
	DeprecationReason string `json:"deprecationReason"`
}

type IntrospectionInputValue struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	DefaultValue string               `json:"defaultValue"`
	Type         IntrospectionTypeRef `json:"type"`
}

type IntrospectionTypeRef struct {
	Kind   string                `json:"kind"`
	Name   string                `json:"name"`
	OfType *IntrospectionTypeRef `json:"ofType"`
}

// IntrospectionQuery is the query that is fired at an API to reconstruct its schema
var IntrospectionQuery = `
	query IntrospectionQuery {
		__schema {
			queryType { name }
			mutationType { name }
			subscriptionType { name }
			types {
				...FullType
			}
			directives {
				name
				description
				locations
				args {
				...InputValue
				}
			}
		}
	}

	fragment FullType on __Type {
		kind
		name
		description
		fields(includeDeprecated: true) {
			name
			description
			args {
				...InputValue
			}
			type {
				...TypeRef
			}
			isDeprecated
			deprecationReason
		}

		inputFields {
			...InputValue
		}

		interfaces {
			...TypeRef
		}

		enumValues(includeDeprecated: true) {
			name
			description
			isDeprecated
			deprecationReason
		}
		possibleTypes {
			...TypeRef
		}
	}

	fragment InputValue on __InputValue {
		name
		description
		type { ...TypeRef }
		defaultValue
	}

	fragment TypeRef on __Type {
		kind
		name
		ofType {
			kind
			name
			ofType {
				kind
				name
				ofType {
					kind
					name
					ofType {
						kind
						name
						ofType {
							kind
							name
							ofType {
								kind
								name
								ofType {
									kind
									name
								}
							}
						}
					}
				}
			}
		}
	}
`
