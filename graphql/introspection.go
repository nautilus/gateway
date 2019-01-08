package graphql

import (
	"errors"
	"fmt"

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
	schema := &ast.Schema{}

	// if we dont have a name on the response
	if remoteSchema.QueryType.Name == "" {
		return nil, errors.New("Could not find the root query")
	}

	// add each type to the schema
	for _, remoteType := range remoteSchema.Types {
		// convert turn the API payload into a schema type
		schemaType := introspectionUnmarshalType(remoteType)

		// check if this type is the QueryType
		fmt.Println(remoteType.Name, remoteSchema.QueryType.Name)
		if remoteType.Name == remoteSchema.QueryType.Name {
			schema.Query = schemaType
		} else if remoteSchema.MutationType != nil && schemaType.Name == remoteSchema.MutationType.Name {
			schema.Mutation = schemaType
		}
	}

	// we're done here
	return schema, nil
}

func introspectionUnmarshalType(schemaType IntrospectionQueryFullType) *ast.Definition {
	return &ast.Definition{
		Name: schemaType.Name,
	}
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
	EnumValues []struct {
		Name              string `json:"name"`
		Description       string `json:"description"`
		IsDeprecated      bool   `json:"isDeprecated"`
		DeprecationReason string `json:"deprecationReason"`
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
	Kind          string                            `json:"kind"`
	Name          string                            `json:"name"`
	Description   string                            `json:"description"`
	InputFields   []IntrospectionInputValue         `json:"inputField"`
	Interfaces    []IntrospectionTypeRef            `json:"interfaces"`
	PossibleTypes []IntrospectionTypeRef            `json:"possibleTypes"`
	Fields        []IntrospectionQueryFullTypeField `json:"fields"`
}

type IntrospectionInputValue struct {
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	DefaultValue string                `json:"defaultValue"`
	Type         *IntrospectionTypeRef `json:"type"`
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
