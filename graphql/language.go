package graphql

import (
	"fmt"

	"github.com/vektah/gqlparser/ast"
)

// CollectedField is a representations of a field with the list of selection sets that
// must be merged under that field.
type CollectedField struct {
	*ast.Field
	NestedSelections []ast.SelectionSet
}

// CollectedFieldList is a list of CollectedField with utilities for retrieving them
type CollectedFieldList []*CollectedField

func (c *CollectedFieldList) GetOrCreateForAlias(alias string, creator func() *CollectedField) *CollectedField {
	// look for the field with the given alias
	for _, field := range *c {
		if field.Alias == alias {
			return field
		}
	}

	// if we didn't find a field with the chosen alias
	new := creator()

	// add the new field to the list
	*c = append(*c, new)

	return new
}

// ApplyFragments takes a list of selections and merges them into one, embedding any fragments it
// runs into along the way
func ApplyFragments(selectionSet ast.SelectionSet, fragmentDefs ast.FragmentDefinitionList) (ast.SelectionSet, error) {
	// build up a list of selection sets
	final := ast.SelectionSet{}

	// look for all of the collected fields
	CollectedFields, err := collectFields([]ast.SelectionSet{selectionSet}, fragmentDefs)
	if err != nil {
		return nil, err
	}

	// the final result of collecting fields should have a single selection in its selection set
	// which should be a selection for the same field referenced by collected.Field
	for _, collected := range *CollectedFields {
		final = append(final, collected.Field)
	}

	return final, nil
}

func collectFields(sources []ast.SelectionSet, fragments ast.FragmentDefinitionList) (*CollectedFieldList, error) {
	// a way to look up field definitions and the list of selections we need under that field
	selectedFields := &CollectedFieldList{}

	// each selection set we have to merge can contribute to selections for each field
	for _, selectionSet := range sources {
		for _, selection := range selectionSet {
			// a selection can be one of 3 things: a field, a fragment reference, or an inline fragment
			switch selection := selection.(type) {

			// a selection could either have a collected field or a real one. either way, we need to add the selection
			// set to the entry in the map
			case *ast.Field, *CollectedField:
				var selectedField *ast.Field
				if field, ok := selection.(*ast.Field); ok {
					selectedField = field
				} else if collected, ok := selection.(*CollectedField); ok {
					selectedField = collected.Field
				}

				// look up the entry in the field list for this field
				collected := selectedFields.GetOrCreateForAlias(selectedField.Alias, func() *CollectedField {
					return &CollectedField{Field: selectedField}
				})

				// add the fields selection set to the list
				collected.NestedSelections = append(collected.NestedSelections, selectedField.SelectionSet)

			// fragment selections need to be unwrapped and added to the final selection
			case *ast.InlineFragment, *ast.FragmentSpread:
				var selectionSet ast.SelectionSet

				// inline fragments
				if inlineFragment, ok := selection.(*ast.InlineFragment); ok {
					selectionSet = inlineFragment.SelectionSet

					// fragment spread
				} else if fragment, ok := selection.(*ast.FragmentSpread); ok {
					// grab the definition for the fragment
					definition := fragments.ForName(fragment.Name)
					if definition == nil {
						// this shouldn't happen since validation has already ran
						return nil, fmt.Errorf("Could not find fragment definition: %s", fragment.Name)
					}

					selectionSet = definition.SelectionSet
				}

				// fields underneath the inline fragment could be fragments themselves
				fields, err := collectFields([]ast.SelectionSet{selectionSet}, fragments)
				if err != nil {
					return nil, err
				}

				// each field in the inline fragment needs to be added to the selection
				for _, fragmentSelection := range *fields {
					// add the selection from the field to our accumulator
					collected := selectedFields.GetOrCreateForAlias(fragmentSelection.Alias, func() *CollectedField {
						return fragmentSelection
					})

					// add the fragment selection set to the list of selections for the field
					collected.NestedSelections = append(collected.NestedSelections, fragmentSelection.SelectionSet)
				}
			}
		}
	}

	// each selected field needs to be merged into a single selection set
	for _, collected := range *selectedFields {
		// compute the new selection set for this field
		merged, err := collectFields(collected.NestedSelections, fragments)
		if err != nil {
			return nil, err
		}

		// if there are selections for the field we need to turn them into a selection set
		selectionSet := ast.SelectionSet{}
		for _, selection := range *merged {
			selectionSet = append(selectionSet, selection)
		}

		// save this selection set over the nested one
		collected.SelectionSet = selectionSet
	}

	// we're done
	return selectedFields, nil
}

func SelectedFields(source ast.SelectionSet) []*ast.Field {
	// build up a list of fields
	fields := []*ast.Field{}

	// each source could contribute fields to this
	for _, selection := range source {
		// if we are selecting a field
		switch selection := selection.(type) {
		case *ast.Field:
			fields = append(fields, selection)
		case *CollectedField:
			fields = append(fields, selection.Field)
		}
	}

	// we're done
	return fields
}

// ExtractVariables takes a list of arguments and returns a list of every variable used
func ExtractVariables(args ast.ArgumentList) []string {
	// the list of variables
	variables := []string{}

	// each argument could contain variables
	for _, arg := range args {
		extractVariablesFromValues(&variables, arg.Value)
	}

	// return the list
	return variables
}

func extractVariablesFromValues(accumulator *[]string, value *ast.Value) {
	// we have to look out for a few different kinds of values
	switch value.Kind {
	// if the value is a reference to a variable
	case ast.Variable:
		// add the ference to the list
		*accumulator = append(*accumulator, value.Raw)
	// the value could be a list
	case ast.ListValue, ast.ObjectValue:
		// each entry in the list or object could contribute a variable
		for _, child := range value.Children {
			extractVariablesFromValues(accumulator, child.Value)
		}
	}
}
