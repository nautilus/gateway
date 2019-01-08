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
	Schema IntrospectionQuerySchema `json:"__schema"`
}

type IntrospectionQuerySchema struct {
	QueryType        IntrospectionQueryRootType              `json:"queryType"`
	MutationType     IntrospectionQueryRootType              `json:"mutationType"`
	SubscriptionType IntrospectionQueryRootType              `json:"subscriptionType"`
	Types            []IntrospectionQueryFullType            `json:"types"`
	Directives       []introspectiveQueryDirectiveDefinition `json:"directives"`
}

type IntrospectionQueryRootType struct {
	Name string `json:"name"`
}

type IntrospectionQueryFullType struct {
	Kind string `json:"kind"`
}

type introspectiveQueryDirectiveDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
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
