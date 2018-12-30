package gateway

import (
	"errors"

	"github.com/vektah/gqlparser/ast"
)

// MergeSchemas takes in a bunch of schemas and merges them into one. Types that
// overlapping names are assumed to be contributions to the same type. ie,
//
// type User {
//     firstName: String!
// }
//
// and
//
// type User {
//     lastName: String!
// }
//
// get merged into
//
// type User {
//     firstName: String!
//     lastName: String!
// }
func MergeSchemas(sources []*ast.Schema) (*ast.Schema, error) {
	// a placeholder schema we will build up using the sources
	result := &ast.Schema{
		Types: map[string]*ast.Definition{},
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
						return nil, errors.New("schema merge conflict:" + "Two schemas cannot the same field defined for the same type")
					}

					// its safe to copy over the definition
					previousFields = append(previousFields, newField)
				}

				// copy over the new fields for this type definition
				previousDefinition.Fields = previousFields
			}
		}
	}

	// we're done here
	return result, nil
}
