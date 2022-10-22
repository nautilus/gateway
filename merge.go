package gateway

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// Merger is an interface for structs that are capable of taking a list of schemas and returning something that resembles
// a "merge" of those schemas.
type Merger interface {
	Merge([]*ast.Schema) (*ast.Schema, error)
}

// MergerFunc is a wrapper of a function of the same signature as Merger.Merge
type MergerFunc func([]*ast.Schema) (*ast.Schema, error)

// Merge invokes and returns the wrapped function
func (m MergerFunc) Merge(sources []*ast.Schema) (*ast.Schema, error) {
	return m(sources)
}

// mergeSchemas takes in a bunch of schemas and merges them into one. Following the strategies outlined here:
// https://github.com/nautilus/gateway/blob/master/docs/mergingStrategies.md
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

			previousDefinition, err := mergeInterfaces(previousDefinition, definition)
			if err != nil {
				return nil, err
			}
			result.Types[name] = previousDefinition
		}
	}

	possibleTypesSet := map[string]Set{}

	// merge each definition of each type into one
	for name, definitions := range types {
		if _, exists := possibleTypesSet[name]; !exists {
			possibleTypesSet[name] = Set{}
		}
		for _, definition := range definitions {
			// look up if the type is already registered in the aggregate
			previousDefinition, exists := result.Types[name]

			// if we haven't seen it before
			if !exists {
				// use the declaration that we got from the new schema
				result.Types[name] = definition

				if definition.Kind == ast.Union {
					for _, possibleType := range definition.Types {
						for _, typedef := range types[possibleType] {
							if !possibleTypesSet[name].Has(typedef.Name) {
								possibleTypesSet[name].Add(typedef.Name)
								result.AddPossibleType(name, typedef)
							}
						}
					}
				} else {
					// register the type as an implementer of itself
					result.AddPossibleType(name, definition)
				}

				// each interface that this type implements needs to be registered
				for _, iface := range definition.Interfaces {
					result.AddPossibleType(iface, definition)
					result.AddImplements(definition.Name, result.Types[definition.Name])
				}

				// we're done with this type
				continue
			}

			// we only want one copy of the internal stuff
			if strings.HasPrefix(definition.Name, "__") {
				continue
			}

			// unify handling of errors for merging
			var err error

			switch definition.Kind {
			case ast.Object:
				previousDefinition, err = mergeObjectTypes(previousDefinition, definition)
			case ast.Interface:
				previousDefinition, err = mergeInterfaces(previousDefinition, definition)
			case ast.InputObject:
				previousDefinition, err = mergeInputObjects(previousDefinition, definition)
			case ast.Enum:
				previousDefinition, err = mergeEnums(previousDefinition, definition)
			case ast.Scalar:
				previousDefinition, err = mergeScalars(previousDefinition, definition)
			case ast.Union:
				previousDefinition, err = mergeUnions(previousDefinition, definition)
			}

			if err != nil {
				return nil, err
			}
			result.Types[name] = previousDefinition
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
			previousDefinition, err := mergeDirectives(previousDefinition, definition)
			if err != nil {
				return nil, err
			}
			result.Directives[name] = previousDefinition
		}
	}

	// for now, just use the query type as the query type
	queryType := result.Types[typeNameQuery]
	mutationType := result.Types[typeNameMutation]
	subscriptionType := result.Types[typeNameSubscription]

	result.Query = queryType
	result.Mutation = mutationType
	result.Subscription = subscriptionType

	// we're done here
	return result, nil
}

func mergeInterfaces(previousDefinition *ast.Definition, newDefinition *ast.Definition) (*ast.Definition, error) {
	prevCopy := *previousDefinition
	// descriptions
	if prevCopy.Description == "" {
		prevCopy.Description = newDefinition.Description
	}

	// fields
	if len(previousDefinition.Fields) != len(newDefinition.Fields) {
		return nil, fmt.Errorf("inconsistent number of fields")
	}
	prevCopy.Fields = append(ast.FieldList{}, previousDefinition.Fields...)
	for ix, field := range prevCopy.Fields {
		// get the corresponding field in the other definition
		otherField := newDefinition.Fields.ForName(field.Name)

		var err error
		prevCopy.Fields[ix], err = mergeFields(field, otherField)
		if err != nil {
			return nil, fmt.Errorf("encountered error merging interface %v: %w", previousDefinition.Name, err)
		}
	}

	return &prevCopy, nil
}

func mergeObjectTypes(previousDefinition *ast.Definition, newDefinition *ast.Definition) (*ast.Definition, error) {
	prevCopy := *previousDefinition
	// descriptions
	if prevCopy.Description == "" {
		prevCopy.Description = newDefinition.Description
	}

	// interfaces
	prevCopy.Interfaces = mergeInterfaceNames(prevCopy.Interfaces, newDefinition.Interfaces)

	// we have to add the fields in the source definition with the one in the aggregate
	prevCopy.Fields = append(ast.FieldList{}, previousDefinition.Fields...)
	for _, newField := range newDefinition.Fields {
		// look up if we already know about this field
		prevIndex, prevField := findField(prevCopy.Fields, newField.Name)
		// if we already know about the field
		if prevField != nil {
			// and they aren't equal
			var err error
			prevCopy.Fields[prevIndex], err = mergeFields(prevField, newField)
			if err != nil {
				//  we don't allow 2 fields that have different types
				return nil, fmt.Errorf("encountered error merging object %v: %w", previousDefinition.Name, err)
			}
		} else {
			// its safe to copy over the definition
			prevCopy.Fields = append(prevCopy.Fields, newField)
		}
	}

	// make sure that the 2 directive lists are the same
	if err := mergeDirectiveListsEqual(previousDefinition.Directives, newDefinition.Directives); err != nil {
		return nil, err
	}

	return &prevCopy, nil
}

func mergeInterfaceNames(interfaces1, interfaces2 []string) []string {
	interfacesSet := make(map[string]struct{})
	for _, i := range interfaces1 {
		interfacesSet[i] = struct{}{}
	}
	for _, i := range interfaces2 {
		interfacesSet[i] = struct{}{}
	}
	var result []string
	for i := range interfacesSet {
		result = append(result, i)
	}
	sort.Strings(result)
	return result
}

func findField(fields ast.FieldList, fieldName string) (int, *ast.FieldDefinition) {
	for ix, field := range fields {
		if field.Name == fieldName {
			return ix, field
		}
	}
	return -1, nil
}

func mergeInputObjects(object1, object2 *ast.Definition) (*ast.Definition, error) {
	object1Copy := *object1

	// if the field list isn't the same
	var err error
	object1Copy.Fields, err = mergeFieldList(object1.Fields, object2.Fields)
	if err != nil {
		return nil, err
	}

	// check directives
	if err := mergeDirectiveListsEqual(object1.Directives, object2.Directives); err != nil {
		return nil, err
	}

	return object1, nil
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

func mergeEnums(previousDefinition *ast.Definition, newDefinition *ast.Definition) (*ast.Definition, error) {
	prevCopy := *previousDefinition

	// if we are merging an internal enums
	if strings.HasPrefix(previousDefinition.Name, "__") {
		// let it through without changing
		return &prevCopy, nil
	}

	if prevCopy.Description == "" {
		prevCopy.Description = newDefinition.Description
	}

	// if the two definitions dont have the same length
	if len(previousDefinition.EnumValues) != len(newDefinition.EnumValues) {
		return nil, fmt.Errorf("enum %s has an inconsistent definition in different services", newDefinition.Name)
	}
	// a set of values
	for ix, value := range prevCopy.EnumValues {
		// look up the valuein the new definition
		newValue := newDefinition.EnumValues.ForName(value.Name)

		var err error
		prevCopy.EnumValues[ix], err = mergeEnumValues(value, newValue)
		if err != nil {
			return nil, err
		}
	}

	// we're done
	return &prevCopy, nil
}

func mergeUnions(previousDefinition *ast.Definition, newDefinition *ast.Definition) (*ast.Definition, error) {
	// unions are defined by a list of strings that name the sub types

	// if the length of the 2 lists is not the same
	if len(previousDefinition.Types) != len(newDefinition.Types) {
		return nil, fmt.Errorf("union %s did not have a consistent number of sub types", previousDefinition.Name)
	}

	if err := mergeStringSliceEquivalent(previousDefinition.Types, newDefinition.Types); err != nil {
		return nil, err
	}

	// nothing is wrong
	return previousDefinition, nil
}

func mergeDirectives(previousDefinition *ast.DirectiveDefinition, newDefinition *ast.DirectiveDefinition) (*ast.DirectiveDefinition, error) {
	result := *previousDefinition // shallow copy to mutate merge result
	// keep the first description
	if result.Description == "" {
		result.Description = newDefinition.Description
	}

	// make sure the 2 directives can be placed on the same locations
	var err error
	result.Locations, err = mergeDirectiveLocations(result.Locations, newDefinition.Locations)
	if err != nil {
		return nil, fmt.Errorf("conflict in locations for directive %s: %w", previousDefinition.Name, err)
	}

	// make sure the 2 definitions take the same arguments
	result.Arguments, err = mergeArgumentDefinitionList(result.Arguments, newDefinition.Arguments, result.Position.Src.BuiltIn)
	if err != nil {
		return nil, fmt.Errorf("conflict in argument definitions for directive %s: %w", previousDefinition.Name, err)
	}

	// the 2 directives can coexist
	return &result, nil
}

func mergeEnumValues(value1, value2 *ast.EnumValueDefinition) (*ast.EnumValueDefinition, error) {
	value1Copy := *value1
	if value1Copy.Description == "" {
		value1Copy.Description = value2.Description
	}

	// if the 2 directives dont match
	if err := mergeDirectiveListsEqual(value1.Directives, value2.Directives); err != nil {
		return nil, fmt.Errorf("conflict in enum value directives: %w", err)
	}

	return &value1Copy, nil
}

func mergeScalars(value1, value2 *ast.Definition) (*ast.Definition, error) {
	value1Copy := *value1
	if value1Copy.Description == "" {
		value1Copy.Description = value2.Description
	}

	// if the 2 directives dont match
	if err := mergeDirectiveListsEqual(value1.Directives, value2.Directives); err != nil {
		return nil, fmt.Errorf("conflict in enum value directives: %w", err)
	}

	return &value1Copy, nil
}

func mergeFieldList(list1, list2 ast.FieldList) (ast.FieldList, error) {
	if len(list1) != len(list2) {
		return nil, fmt.Errorf("inconsistent number of fields")
	}

	var list1Copy ast.FieldList
	for _, field := range list1 {
		// get the corresponding field in the other definition
		otherField := list2.ForName(field.Name)
		if otherField == nil {
			return nil, fmt.Errorf("could not find field %s", field.Name)
		}

		newField, err := mergeFields(field, otherField)
		if err != nil {
			return nil, err
		}
		list1Copy = append(list1Copy, newField)
	}

	return list1Copy, nil
}

func mergeFields(field1, field2 *ast.FieldDefinition) (*ast.FieldDefinition, error) {
	field1Copy := *field1
	// descriptions
	if field1Copy.Description == "" {
		field1Copy.Description = field2.Description
	}

	// fields
	if err := mergeTypesEqual(field1.Type, field2.Type); err != nil {
		return nil, fmt.Errorf("fields are not equal: %w", err)
	}

	// arguments
	var err error
	field1Copy.Arguments, err = mergeArgumentDefinitionList(field1.Arguments, field2.Arguments, false)
	if err != nil {
		return nil, fmt.Errorf("fields are not equal: %w", err)
	}

	// default values
	if err := mergeValuesEqual(field1.DefaultValue, field2.DefaultValue); err != nil {
		return nil, fmt.Errorf("fields are not equal: %w", err)
	}

	// directives
	if err := mergeDirectiveListsEqual(field1.Directives, field2.Directives); err != nil {
		return nil, fmt.Errorf("fields are not equal: %w", err)
	}

	// nothing went wrong
	return &field1Copy, nil
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

func mergeArgumentDefinitionList(list1, list2 ast.ArgumentDefinitionList, ignoreNewDefaultValue bool) (ast.ArgumentDefinitionList, error) {
	list1Copy := append(ast.ArgumentDefinitionList{}, list1...)
	// if the 2 lists are not the same length
	if len(list1) != len(list2) {
		// they will never be the same
		return nil, errors.New("there were an inconsistent number of arguments")
	}

	// compare each argument to its counterpart in the other list
	for ix, arg1 := range list1Copy {
		arg2 := list2.ForName(arg1.Name)
		if arg2 == nil {
			return nil, fmt.Errorf("could not find the argument with name %s", arg1.Name)
		}

		// if the 2 arguments are not the same
		var err error
		list1Copy[ix], err = mergeArgumentDefinitions(arg1, arg2, ignoreNewDefaultValue)
		if err != nil {
			return nil, err
		}
	}

	return list1Copy, nil
}

func mergeArgumentDefinitions(prevArg *ast.ArgumentDefinition, newArg *ast.ArgumentDefinition, ignoreNewDefaultValue bool) (*ast.ArgumentDefinition, error) {
	result := *prevArg
	// descriptions
	if result.Description == "" {
		result.Description = newArg.Description
	}

	// check that the 2 types are equal
	if err := mergeTypesEqual(result.Type, newArg.Type); err != nil {
		return nil, err
	}

	// check that the 2 default values are equal
	if !ignoreNewDefaultValue { // use first arg's default value and ignore others, e.g. built-in directive argument default values
		if err := mergeValuesEqual(result.DefaultValue, newArg.DefaultValue); err != nil {
			return nil, err
		}
	}

	return &result, nil
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

// Directives can be used on execution locations (a query) or on type system locations (a deprecated field).
// Gateway should not merge and share execution locations since these may not be supported by the respective service.
// Gateway should merge and share type system locations since these are only defined and used in each respective service's schema. In other words, the services own their usage of those directives.
//
// Some implementations merge all locations irrespective of their kind. This could result in a
// runtime error or an ignored execution directive instead of an immediate query syntax error.
//
// From the spec: http://spec.graphql.org/October2021/#DirectiveLocation
//
//	DirectiveLocation :
//		ExecutableDirectiveLocation
//		TypeSystemDirectiveLocation
//	ExecutableDirectiveLocation :
//	  QUERY
//	  MUTATION
//	  SUBSCRIPTION
//	  FIELD
//	  FRAGMENT_DEFINITION
//	  FRAGMENT_SPREAD
//	  INLINE_FRAGMENT
//	  VARIABLE_DEFINITION
//	TypeSystemDirectiveLocation :
//	  SCHEMA
//	  SCALAR
//	  OBJECT
//	  FIELD_DEFINITION
//	  ARGUMENT_DEFINITION
//	  INTERFACE
//	  UNION
//	  ENUM
//	  ENUM_VALUE
//	  INPUT_OBJECT
//	  INPUT_FIELD_DEFINITION
func mergeDirectiveLocations(list1, list2 []ast.DirectiveLocation) ([]ast.DirectiveLocation, error) {
	resultSet := make(map[ast.DirectiveLocation]struct{})
	executableSet1 := make(map[ast.DirectiveLocation]struct{})
	// Check the permissive set (type system locations) rather than the restrictive set (executable locations).
	// The kinds of locations can expand in future versions of the spec, so we should err on the side
	// of denying new type system fields instead of allowing new executable fields.
	for _, l := range list1 {
		resultSet[l] = struct{}{}
		if !isTypeSystemDirectiveLocation(l) {
			executableSet1[l] = struct{}{}
		}
	}
	executableSet2 := make(map[ast.DirectiveLocation]struct{})
	for _, l := range list2 {
		resultSet[l] = struct{}{}
		if !isTypeSystemDirectiveLocation(l) {
			executableSet2[l] = struct{}{}
		}
	}

	mismatchErr := fmt.Errorf("do not have the same executable locations: %s", executableDirectiveLocationDiff(executableSet1, executableSet2))
	for l := range executableSet1 {
		if _, ok := executableSet2[l]; !ok {
			return nil, mismatchErr
		}
	}
	for l := range executableSet2 {
		if _, ok := executableSet1[l]; !ok {
			return nil, mismatchErr
		}
	}

	var result []ast.DirectiveLocation
	for l := range resultSet {
		result = append(result, l)
	}
	sort.Slice(result, func(a, b int) bool {
		return result[a] < result[b]
	})
	return result, nil
}

func isTypeSystemDirectiveLocation(d ast.DirectiveLocation) bool {
	switch d {
	case
		ast.LocationSchema,
		ast.LocationScalar,
		ast.LocationObject,
		ast.LocationFieldDefinition,
		ast.LocationArgumentDefinition,
		ast.LocationInterface,
		ast.LocationUnion,
		ast.LocationEnum,
		ast.LocationEnumValue,
		ast.LocationInputObject,
		ast.LocationInputFieldDefinition:
		return true
	case
		ast.LocationQuery,
		ast.LocationMutation,
		ast.LocationSubscription,
		ast.LocationField,
		ast.LocationFragmentDefinition,
		ast.LocationFragmentSpread,
		ast.LocationInlineFragment,
		ast.LocationVariableDefinition:
		return false
	default:
		return false
	}
}

func executableDirectiveLocationDiff(set1, set2 map[ast.DirectiveLocation]struct{}) string {
	var diff []string
	for l := range set1 {
		if _, ok := set2[l]; !ok {
			diff = append(diff, string(l))
		}
	}
	return fmt.Sprintf("these locations are not shared: %s", strings.Join(diff, ", "))
}
