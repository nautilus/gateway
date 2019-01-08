package graphql

import (
	"github.com/vektah/gqlparser/ast"
)

// IntrospectAPI send the introspection query to a Queryer and builds up the
// schema object described by the result
func IntrospectAPI(queryer Queryer) *ast.Schema {
	return nil
}

type IntrospectionQueryResult struct {
	Schema *IntrospectionQuerySchema `json:"__schema"`
}

type IntrospectionQuerySchema struct {
	QueryType        *IntrospectionQueryRootType  `json:"queryType"`
	MutationType     *IntrospectionQueryRootType  `json:"mutationType"`
	SubscriptionType *IntrospectionQueryRootType  `json:"subscriptionType"`
	Types            []IntrospectionQueryFullType `json:"types"`
	Directives       []struct {
		Name        string                    `json:"name"`
		Description string                    `json:"description"`
		Locations   []string                  `json:"locations"`
		Args        []IntrospectionInputValue `json:"args"`
	} `json:"directives"`
	EnumValues []struct {
		Name              string `json:"name"`
		Description       string `json:"description"`
		IsDeprecated      bool   `json:"isDeprecated"`
		DeprecationReason string `json:"deprecationReason"`
	} `json:"enumValues"`
}

type IntrospectionQueryRootType struct {
	Name string `json:"name"`
}

type IntrospectionQueryFullType struct {
	Kind          string                    `json:"kind"`
	Name          string                    `json:"name"`
	Description   string                    `json:"description"`
	InputFields   []IntrospectionInputValue `json:"inputFields"`
	Interfaces    []IntrospectionTypeRef    `json:"interfaces"`
	PossibleTypes []IntrospectionTypeRef    `json:"possibleTypes"`
	Fields        []struct {
		Name              string                    `json:"name"`
		Description       string                    `json:"description"`
		Args              []IntrospectionInputValue `json:"args"`
		Type              *IntrospectionTypeRef     `json:"type"`
		IsDeprecated      bool                      `json:"isDeprecated"`
		DeprecationReason string                    `json:"deprecationReason"`
	} `json:"fields"`
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

// IntrospectionQueryContent is the query that is fired at an API to reconstruct its schema
var IntrospectionQueryContent = `
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
		.	..TypeRef
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
