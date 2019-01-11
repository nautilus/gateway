package gateway

import (
	"errors"
	"fmt"

	"github.com/vektah/gqlparser/ast"
)

// mergeSchemas takes in a bunch of schemas and merges them into one. Following the strategies outlined here:
// https://github.com/AlecAivazis/graphql-gateway/blob/master/docs/mergingStrategies.md
func mergeSchemas(sources []*ast.Schema) (*ast.Schema, error) {
	// a placeholder schema we will build up using the sources
	result := &ast.Schema{
		Types:         map[string]*ast.Definition{},
		PossibleTypes: map[string][]*ast.Definition{},
		Implements:    map[string][]*ast.Definition{},
	}

	// we have to visit each source schema
	for _, schema := range sources {
		// add each type declared by the source schema to the one we are building up
		for name, newDefinition := range schema.Types {
			// look up if the type is already registered in the aggregate
			previousDefinition, exists := result.Types[name]

			// if we haven't seen it before
			if !exists {
				// use the declaration that we got from the new schema
				result.Types[name] = newDefinition

				// register the type as an implementer of itself
				result.AddPossibleType(name, newDefinition)

				// we are merging the same type from 2 different schemas together
			} else {
				// the fields in the aggregate
				previousFields := previousDefinition.Fields

				// we have to add the fields in the source definition with the one in the aggregate
				for _, newField := range newDefinition.Fields {
					// look up if we already know about this field
					field := previousFields.ForName(newField.Name)
					// if we already have that field defined and it has a different type and the one from the source schema
					if field != nil && field.Type.String() != newField.Type.String() {
						log.Warn(fmt.Sprintf("Could not merge schemas together. Conflicting definitions of %s", field.Name))
						return nil, errors.New("schema merge conflict: Two schemas cannot the same field defined for the same type")
					}

					// its safe to copy over the definition
					previousFields = append(previousFields, newField)
				}

				// copy over the new fields for this type definition
				previousDefinition.Fields = previousFields
			}
		}
	}

	// for now, just use the query type as the query type
	queryType, _ := result.Types["Query"]
	mutationType, _ := result.Types["Mutation"]
	subscriptionType, _ := result.Types["Subscription"]

	result.Query = queryType
	result.Mutation = mutationType
	result.Subscription = subscriptionType

	// we're done here
	return result, nil
}
