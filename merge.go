package gateway

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/ast"
)

// Merger is an interface for structs that are capable of taking a list of schemas and returning something that resembles
// a "merge" of those schemas.
type Merger interface {
	Merge([]*ast.Schema) (*ast.Schema, error)
}

// MergerFn is a wrapper of a function of the same signature as Merger.Merge
type MergerFn struct {
	Fn func([]*ast.Schema) (*ast.Schema, error)
}

// Merge invokes and returns the wrapped function
func (m *MergerFn) Merge(sources []*ast.Schema) (*ast.Schema, error) {
	return m.Fn(sources)
}

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

	// merging the schemas has to happen in 2 passes per definnition. The first groups definitions into different
	// categories. Interfaces need to be validated before we can add the types that implement them
	types := map[string][]*ast.Definition{}
	directives := map[string][]*ast.DirectiveDefinition{}
	interfaces := map[string][]*ast.Definition{}

	// we have to visit each source schema
	for _, schema := range sources {
		// add each type declared by the source schema to the one we are building up
		for name, definition := range schema.Types {
			// if the definition is not an interface
			if definition.Kind != ast.Interface {
				types[name] = append(types[name], definition)
			} else {
				interfaces[name] = append(interfaces[name], definition)
			}
		}

		// add each directive to the list
		for name, definition := range schema.Directives {
			directives[name] = append(directives[name], definition)
		}
	}

	// merge each definition of each type into one
	for name, definitions := range types {
		// merge each definition together
		for _, definition := range definitions {
			// look up if the type is already registered in the aggregate
			previousDefinition, exists := result.Types[name]

			// if we haven't seen it before
			if !exists {
				// use the declaration that we got from the new schema
				result.Types[name] = definition

				// register the type as an implementer of itself
				result.AddPossibleType(name, definition)

				// we're done with this type
				continue
			}

			// unify handling of errors for merging
			var err error

			if len(definition.Fields) > 0 {
				// if the definition is an object or input object we have to merge it
				err = mergeObjectTypes(previousDefinition, definition)

			} else if len(definition.EnumValues) > 0 {
				// the definition is an enum value
				err = mergeEnums(previousDefinition, definition)
			}

			if err != nil {
				return nil, err
			}
		}
	}

	// for each directive
	for name, definitions := range directives {
		// merge each definition together
		for _, definition := range definitions {
			// look up if the type is already registered in the aggregate
			previousDefinition, exists := result.Directives[name]

			// if we haven't seen it before
			if !exists {
				// use the declaration that we got from the new schema
				result.Directives[name] = definition

				// we're done with this type
				continue
			}

			// we have to merge the 2 directives
			err := mergeDirectives(previousDefinition, definition)
			if err != nil {
				return nil, err
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
	// if we are merging an internal enums
	if strings.HasPrefix(previousDefinition.Name, "__") {
		// let it through without changing
		return nil
	}

	return fmt.Errorf("enum %s cannot be split across services", newDefinition.Name)
}

func mergeDirectives(previousDefinition *ast.DirectiveDefinition, newDefinition *ast.DirectiveDefinition) error {
	// currently, the only meaning to merging directives is to ignore the second one as long as it has the same definition
	// as the first

	// if the 2 descriptions don't match
	if previousDefinition.Description != newDefinition.Description {
		return fmt.Errorf("conflict in directive descriptions: \"%v\" and \"%v\"", previousDefinition.Description, newDefinition.Description)
	}

	// make sure the 2 definitions take the same arguments
	if err := mergeArgumentListEqual(previousDefinition.Arguments, newDefinition.Arguments); err != nil {
		return fmt.Errorf("conflict in argument definitions for directive %s. %s", previousDefinition.Name, err.Error())
	}

	// make sure the 2 directives can be placed on the same locations
	if err := mergeDirectiveLocationsEqual(previousDefinition.Locations, newDefinition.Locations); err != nil {
		return fmt.Errorf("conflict in locations for directive %s. %s", previousDefinition.Name, err.Error())
	}

	// the 2 directives can coexist
	return nil
}

func mergeArgumentListEqual(list1, list2 ast.ArgumentDefinitionList) error {
	// if the 2 lists are not the same length
	if len(list1) != len(list2) {
		// they will never be the same
		return errors.New("there were an inconsistent number of arguments")
	}

	// compare each argument to its counterpart in the other list
	for _, arg1 := range list1 {
		arg2 := list2.ForName(arg1.Name)

		// if the 2 arguments are not the same
		if err := mergeArgumentsEqual(arg1, arg2); err != nil {
			return err
		}
	}

	return nil
}

func mergeArgumentsEqual(arg1 *ast.ArgumentDefinition, arg2 *ast.ArgumentDefinition) error {
	// if the 2 descriptions are not the same
	if arg1.Description != arg2.Description {
		return errors.New("descriptions were not the same")
	}

	// check that the 2 types are equal
	if err := mergeTypesEqual(arg1.Type, arg2.Type); err != nil {
		return err
	}

	// check that the 2 default values are equal
	if err := mergeValuesEqual(arg1.DefaultValue, arg2.DefaultValue); err != nil {
		return err
	}

	return nil
}

func mergeValuesEqual(value1, value2 *ast.Value) error {
	// if one is null and the other isn't
	if (value1 == nil && value2 != nil) || (value1 != nil && value2 == nil) {
		return errors.New("one is a list the other isn't")
	}

	// if they are both nil values, there's no error
	if value1 == nil {
		return nil
	}

	// if the kinds are not the same
	if value1.Kind != value2.Kind {
		return errors.New("encountered inconsistent kinds")
	}

	// if the raw values are not the same
	if value1.Raw != value2.Raw {
		return errors.New("encountered different raw values")
	}

	return nil
}

func mergeTypesEqual(type1, type2 *ast.Type) error {
	// if one is null and the other isn't
	if (type1 == nil && type2 != nil) || (type1 != nil && type2 == nil) {
		return errors.New("one is a list the other isn't")
	}

	// if they are both nil types, there's no error
	if type1 == nil {
		return nil
	}

	// name
	if type1.NamedType != type2.NamedType {
		return errors.New("types do not have the same name")
	}

	// nullability
	if type1.NonNull != type2.NonNull {
		return errors.New("types do not have the same nullability constraints")
	}

	// subtypes (ie, non-null string)
	if err := mergeTypesEqual(type1.Elem, type2.Elem); err != nil {
		return err
	}

	// they're equal
	return nil
}

func mergeDirectiveLocationsEqual(list1, list2 []ast.DirectiveLocation) error {
	// if the 2 lists are not the same length
	if len(list1) != len(list2) {
		// they will never be the same
		return errors.New("do not have the same number of locations")
	}

	// build up a set of the locations for list1
	list1Locs := map[ast.DirectiveLocation]bool{}
	for _, location := range list1 {
		list1Locs[location] = true
	}

	// make sure every location in list2 is there
	for _, location := range list2 {
		// if its not then the 2 lists are different
		if _, ok := list1Locs[location]; !ok {
			return fmt.Errorf("directive could be found on %s in one definition but not the other", location)
		}
	}

	// build a set of the locations for the
	return nil
}
