package graphql

import (
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

	fmt.Println(result)

	return nil, nil
}

type IntrospectionQueryResult struct {
	Schema *IntrospectionQuerySchema `mapstructure:"__schema"`
}

type IntrospectionQuerySchema struct {
	QueryType        *IntrospectionQueryRootType
	MutationType     *IntrospectionQueryRootType
	SubscriptionType *IntrospectionQueryRootType
	Types            []IntrospectionQueryFullType
	Directives       []struct {
		Name        string
		Description string
		Locations   []string
		Args        []IntrospectionInputValue
	}
	EnumValues []struct {
		Name              string
		Description       string
		IsDeprecated      bool
		DeprecationReason string
	}
}

type IntrospectionQueryRootType struct {
	Name string
}

type IntrospectionQueryFullType struct {
	Kind          string
	Name          string
	Description   string
	InputFields   []IntrospectionInputValue
	Interfaces    []IntrospectionTypeRef
	PossibleTypes []IntrospectionTypeRef
	Fields        []struct {
		Name              string
		Description       string
		Args              []IntrospectionInputValue
		Type              *IntrospectionTypeRef
		IsDeprecated      bool
		DeprecationReason string
	}
}

type IntrospectionInputValue struct {
	Name         string
	Description  string
	DefaultValue string
	Type         *IntrospectionTypeRef
}

type IntrospectionTypeRef struct {
	Kind   string
	Name   string
	OfType *IntrospectionTypeRef
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
