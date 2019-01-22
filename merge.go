package gateway

import (
	"errors"

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
		Directives:    map[string]*ast.DirectiveDefinition{},
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

				// we're done with this type
				continue
			}

			// unify handling of errors for merging
			var err error

			if len(newDefinition.Fields) > 0 {
				// if the definition is an object or input object we have to merge it
				err = mergeObjectTypes(previousDefinition, newDefinition)

			} else if len(newDefinition.EnumValues) > 0 {
				// the definition is an enum value
				err = mergeEnums(previousDefinition, newDefinition)
			}

			if err != nil {
				log.Warn("Encountered error merging schemas: ", err.Error())
				return nil, err
			}
		}

		// for each directive
		for directiveName, definition := range schema.Directives {
			result.Directives[directiveName] = definition
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

func mergeObjectTypes(previousDefinition *ast.Definition, newDefinition *ast.Definition) error {
	// the fields in the aggregate
	previousFields := previousDefinition.Fields

	// we have to add the fields in the source definition with the one in the aggregate
	for _, newField := range newDefinition.Fields {
		// look up if we already know about this field
		field := previousFields.ForName(newField.Name)
		// if we already have that field defined and it has a different type and the one from the source schema
		if field != nil && field.Type.String() != newField.Type.String() {
			return errors.New("schema merge conflict: Two schemas cannot the same field defined for the same type")
		}

		// its safe to copy over the definition
		previousFields = append(previousFields, newField)
	}

	// copy over the new fields for this type definition
	previousDefinition.Fields = previousFields

	return nil
}

func mergeEnums(previousDefinition *ast.Definition, newDefinition *ast.Definition) error {
	return errors.New("enums cannot be split across services")
}
