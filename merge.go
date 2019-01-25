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
			// if the definition is an interface
			if definition.Kind == ast.Interface {
				// ad it to the list
				interfaces[name] = append(interfaces[name], definition)
			} else {
				types[name] = append(types[name], definition)
			}
		}

		// add each directive to the list
		for name, definition := range schema.Directives {
			directives[name] = append(directives[name], definition)
		}
	}

	// merge each interface into one
	for name, definitions := range interfaces {
		for _, definition := range definitions {
			// look up if the type is already registered in the aggregate
			previousDefinition, exists := result.Types[name]

			// if we haven't seen it before
			if !exists {
				// use the declaration that we got from the new schema
				result.Types[name] = definition

				result.AddPossibleType(name, definition)

				// we're done with this definition
				continue
			}

			if err := mergeInterfaces(result, previousDefinition, definition); err != nil {
				return nil, err
			}
		}
	}

	// merge each definition of each type into one
	for name, definitions := range types {
		for _, definition := range definitions {
			// look up if the type is already registered in the aggregate
			previousDefinition, exists := result.Types[name]

			// if we haven't seen it before
			if !exists {
				// use the declaration that we got from the new schema
				result.Types[name] = definition

				// register the type as an implementer of itself
				result.AddPossibleType(name, definition)

				// each interface that this type implements needs to be registered
				for _, iface := range definition.Interfaces {
					result.AddPossibleType(iface, definition)
					result.AddImplements(definition.Name, result.Types[definition.Name])
				}

				// we're done with this type
				continue
			}

			// unify handling of errors for merging
			var err error

			switch definition.Kind {
			case ast.Object:
				err = mergeObjectTypes(result, previousDefinition, definition)
			case ast.InputObject:
				err = mergeInputObjects(result, previousDefinition, definition)
			case ast.Enum:
				err = mergeEnums(result, previousDefinition, definition)
			case ast.Union:
				err = mergeUnions(result, previousDefinition, definition)
			}

			if err != nil {
				return nil, err
			}
		}
	}

	// merge each directive definition together
	for name, definitions := range directives {
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
			err := mergeDirectivesEqual(previousDefinition, definition)
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

func mergeInterfaces(schema *ast.Schema, previousDefinition *ast.Definition, newDefinition *ast.Definition) error {
	// descriptions
	if previousDefinition.Description != newDefinition.Description {
		return fmt.Errorf("conflict in interface descriptions: \"%v\" and \"%v\"", previousDefinition.Description, newDefinition.Description)
	}

	// fields
	if len(previousDefinition.Fields) != len(newDefinition.Fields) {
		return fmt.Errorf("inconsistent number of fields")
	}
	for _, field := range previousDefinition.Fields {
		// get the corresponding field in the other definition
		otherField := newDefinition.Fields.ForName(field.Name)

		if err := mergeFieldsEqual(field, otherField); err != nil {
			return fmt.Errorf("encountered error merging interface %v: %v", previousDefinition.Name, err.Error())
		}
	}

	return nil
}

func mergeObjectTypes(schema *ast.Schema, previousDefinition *ast.Definition, newDefinition *ast.Definition) error {
	// the fields in the aggregate
	previousFields := previousDefinition.Fields

	// we have to add the fields in the source definition with the one in the aggregate
	for _, newField := range newDefinition.Fields {
		// look up if we already know about this field
		field := previousFields.ForName(newField.Name)

		// if we already know about the field
		if field != nil {
			// and they aren't equal
			if err := mergeFieldsEqual(field, newField); err != nil {
				//  we don't allow 2 fields that have different types
				return fmt.Errorf("encountered error merging object %v: %v", previousDefinition.Name, err.Error())
			}
		} else {
			// its safe to copy over the definition
			previousFields = append(previousFields, newField)
		}

	}

	// make sure the 2 implement the same number of interfaces
	if err := mergeStringSliceEquivalent(previousDefinition.Interfaces, newDefinition.Interfaces); err != nil {
		return fmt.Errorf("object type does not implement a consistent set of interfaces. %s", err.Error())
	}

	// make sure that the 2 directive lists are the same
	if err := mergeDirectiveListsEqual(previousDefinition.Directives, newDefinition.Directives); err != nil {
		return err
	}

	// copy over the new fields for this type definition
	previousDefinition.Fields = previousFields

	return nil
}

func mergeInputObjects(result *ast.Schema, object1, object2 *ast.Definition) error {
	// if the field list isn't the same
	if err := mergeFieldListEqual(object1.Fields, object2.Fields); err != nil {
		return err
	}

	// check directives
	if err := mergeDirectiveListsEqual(object1.Directives, object2.Directives); err != nil {
		return err
	}

	return nil
}

func mergeStringSliceEquivalent(slice1, slice2 []string) error {
	if len(slice1) != len(slice2) {
		return errors.New("object types have different number of entries")
	}
	if len(slice1) > 0 {
		// build a unique list of every interface
		previousInterfaces := Set{}
		for _, iface := range slice1 {
			previousInterfaces.Add(iface)
		}

		// make sure that the new definition is in the same interfaces
		for _, iface := range slice2 {
			if _, ok := previousInterfaces[iface]; !ok {
				return errors.New("inconsistent values")
			}
		}

	}

	return nil
}

func mergeEnums(schema *ast.Schema, previousDefinition *ast.Definition, newDefinition *ast.Definition) error {
	// if we are merging an internal enums
	if strings.HasPrefix(previousDefinition.Name, "__") {
		// let it through without changing
		return nil
	}

	return fmt.Errorf("enum %s cannot be split across services", newDefinition.Name)
}

func mergeUnions(schema *ast.Schema, previousDefinition *ast.Definition, newDefinition *ast.Definition) error {
	// unions are defined by a list of strings that name the sub types

	// if the length of the 2 lists is not the same
	if len(previousDefinition.Types) != len(newDefinition.Types) {
		return fmt.Errorf("union %s did not have a consistent number of sub types", previousDefinition.Name)
	}

	if err := mergeStringSliceEquivalent(previousDefinition.Types, newDefinition.Types); err != nil {
		return err
	}

	// nothing is wrong
	return nil
}

func mergeDirectivesEqual(previousDefinition *ast.DirectiveDefinition, newDefinition *ast.DirectiveDefinition) error {
	// currently, the only meaning to merging directives is to ignore the second one as long as it has the same definition
	// as the first

	// if the 2 descriptions don't match
	if previousDefinition.Description != newDefinition.Description {
		return fmt.Errorf("conflict in directive descriptions. Found \"%v\" and \"%v\"", previousDefinition.Description, newDefinition.Description)
	}

	// make sure the 2 definitions take the same arguments
	if err := mergeArgumentDefinitionListEqual(previousDefinition.Arguments, newDefinition.Arguments); err != nil {
		return fmt.Errorf("conflict in argument definitions for directive %s. %s", previousDefinition.Name, err.Error())
	}

	// make sure the 2 directives can be placed on the same locations
	if err := mergeDirectiveLocationsEqual(previousDefinition.Locations, newDefinition.Locations); err != nil {
		return fmt.Errorf("conflict in locations for directive %s. %s", previousDefinition.Name, err.Error())
	}

	// the 2 directives can coexist
	return nil
}

func mergeFieldListEqual(list1, list2 ast.FieldList) error {
	if len(list1) != len(list2) {
		return fmt.Errorf("inconsistent number of fields")
	}
	for _, field := range list1 {
		// get the corresponding field in the other definition
		otherField := list2.ForName(field.Name)
		if otherField == nil {
			return fmt.Errorf("could not find field %s", field.Name)
		}

		if err := mergeFieldsEqual(field, otherField); err != nil {
			return err
		}
	}

	return nil
}

func mergeFieldsEqual(field1, field2 *ast.FieldDefinition) error {
	// if the 2 descriptions don't match
	if field1.Description != field2.Description {
		return fmt.Errorf("conflict in field descriptions. Found \"%v\" and \"%v\"", field1.Description, field2.Description)
	}

	// fields
	if err := mergeTypesEqual(field1.Type, field2.Type); err != nil {
		return fmt.Errorf("fields are not equal: %v", err.Error())
	}

	// arguments
	if err := mergeArgumentDefinitionListEqual(field1.Arguments, field2.Arguments); err != nil {
		return fmt.Errorf("fields are not equal: %v", err.Error())
	}

	// default values
	if err := mergeValuesEqual(field1.DefaultValue, field2.DefaultValue); err != nil {
		return fmt.Errorf("fields are not equal: %v", err.Error())
	}

	// directives
	if err := mergeDirectiveListsEqual(field1.Directives, field2.Directives); err != nil {
		return fmt.Errorf("fields are not equal: %v", err.Error())
	}

	// nothing went wrong
	return nil
}

func mergeDirectiveListsEqual(list1, list2 ast.DirectiveList) error {
	// if the 2 lists are not the same length
	if len(list1) != len(list2) {
		// they will never be the same
		return errors.New("there were an inconsistent number of directives")
	}

	// compare each argument to its counterpart in the other list
	for _, arg1 := range list1 {
		arg2 := list2.ForName(arg1.Name)
		if arg2 == nil {
			return fmt.Errorf("could not find the directive with name %s", arg1.Name)
		}

		// if the 2 arguments are not the same
		if err := mergeDirectiveEqual(arg1, arg2); err != nil {
			return err
		}
	}

	return nil
}

func mergeDirectiveEqual(directive1, directive2 *ast.Directive) error {
	// if their names aren't equal
	if directive1.Name != directive2.Name {
		return errors.New("directives do not have the same name")
	}

	// if their list of arguments aren't equal
	if err := mergeArgumentListEqual(directive1.Arguments, directive2.Arguments); err != nil {
		return err
	}

	// if the argumenst
	return nil
}

func mergeArgumentListEqual(list1, list2 ast.ArgumentList) error {
	// if the 2 lists are not the same length
	if len(list1) != len(list2) {
		// they will never be the same
		return errors.New("there were an inconsistent number of arguments")
	}

	// compare each argument to its counterpart in the other list
	for _, arg1 := range list1 {
		arg2 := list2.ForName(arg1.Name)
		if arg2 == nil {
			return fmt.Errorf("could not find the argument with name %s", arg1.Name)
		}

		// if the 2 arguments are not the same
		if err := mergeArgumentsEqual(arg1, arg2); err != nil {
			return err
		}
	}

	return nil
}

func mergeArgumentsEqual(arg1, arg2 *ast.Argument) error {
	// if the names aren't the same
	if arg1.Name != arg2.Name {
		return errors.New("names were not the same")
	}

	// if the values are different
	if err := mergeValuesEqual(arg1.Value, arg2.Value); err != nil {
		return err
	}

	// they're the same
	return nil
}

func mergeArgumentDefinitionListEqual(list1, list2 ast.ArgumentDefinitionList) error {
	// if the 2 lists are not the same length
	if len(list1) != len(list2) {
		// they will never be the same
		return errors.New("there were an inconsistent number of arguments")
	}

	// compare each argument to its counterpart in the other list
	for _, arg1 := range list1 {
		arg2 := list2.ForName(arg1.Name)
		if arg2 == nil {
			return fmt.Errorf("could not find the argument with name %s", arg1.Name)
		}

		// if the 2 arguments are not the same
		if err := mergeArgumentDefinitionsEqual(arg1, arg2); err != nil {
			return err
		}
	}

	return nil
}

func mergeArgumentDefinitionsEqual(arg1 *ast.ArgumentDefinition, arg2 *ast.ArgumentDefinition) error {
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
