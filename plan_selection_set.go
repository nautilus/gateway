package gateway

import (
	"fmt"
	"iter"

	"github.com/vektah/gqlparser/v2/ast"
)

// findSmallestLocationIntersection finds the smallest intersection of selected locations at increasing depth, and stops just before the intersection is empty.
// This is useful for prioritizing parent queries with multiple candidate locations.
func findSmallestLocationIntersection(fragments ast.FragmentDefinitionList, locations FieldURLMap, selectionSet ast.SelectionSet) (uniqueSet[string], error) {
	if err := validateUsedFragmentsAvailable(fragments, selectionSet); err != nil {
		return nil, err
	}

	var intersectingLocations uniqueSet[string]
	for fields := range breadthFirstSelectionSetIterator(fragments, selectionSet) {
		for _, field := range fields {
			selectedLocationsSlice, err := locations.URLFor(field.ObjectDefinition.Name, field.Name)
			if err != nil {
				return nil, err
			}
			selectedLocations := newSet(selectedLocationsSlice)
			if intersectingLocations == nil {
				intersectingLocations = selectedLocations
			}
			newIntersection := intersectingLocations.Intersection(selectedLocations)
			if len(newIntersection) == 0 {
				return intersectingLocations, nil
			}
			intersectingLocations = newIntersection
		}
	}
	return intersectingLocations, nil
}

// validateUsedFragmentsAvailable returns an error if any fragment used in s is not in fragments
func validateUsedFragmentsAvailable(fragments ast.FragmentDefinitionList, s ast.SelectionSet) error {
	for _, selection := range s {
		switch selection := selection.(type) {
		case *ast.Field:
			if err := validateUsedFragmentsAvailable(fragments, selection.SelectionSet); err != nil {
				return err
			}
		case *ast.InlineFragment:
			if err := validateUsedFragmentsAvailable(fragments, selection.SelectionSet); err != nil {
				return err
			}
		case *ast.FragmentSpread:
			fragment := fragments.ForName(selection.Name)
			if fragment == nil {
				return fmt.Errorf("fragment not found: %s", selection.Name)
			}
			if err := validateUsedFragmentsAvailable(fragments, fragment.SelectionSet); err != nil {
				return err
			}
		}
	}
	return nil
}

// breadthFirstSelectionSetIterator iterates over each layer of selectionSet. Top-level fields, next level fields, and so on.
//
// Panics when encountering an undefined fragment. Use [validateFragmentsAvailable] to prevent failures.
func breadthFirstSelectionSetIterator(fragments ast.FragmentDefinitionList, selectionSet ast.SelectionSet) iter.Seq[[]*ast.Field] {
	return func(yield func([]*ast.Field) bool) {
		fields := firstLevelSelectedFields(fragments, selectionSet)
		for len(fields) > 0 {
			if !yield(fields) {
				return
			}
			var subSelection ast.SelectionSet
			for _, field := range fields {
				subSelection = append(subSelection, field.SelectionSet...)
			}
			fields = firstLevelSelectedFields(fragments, subSelection)
		}
	}
}

// firstLevelSelectedFields collects the first layer of ast.Fields, resolving fragments into their respective fields.
//
// Panics when encountering an undefined fragment. Use [validateFragmentsAvailable] to prevent failures.
func firstLevelSelectedFields(fragments ast.FragmentDefinitionList, selectionSet ast.SelectionSet) []*ast.Field {
	var firstLevelSelectionSet []*ast.Field
	for _, selection := range selectionSet {
		switch selection := selection.(type) {
		case *ast.Field:
			firstLevelSelectionSet = append(firstLevelSelectionSet, selection)
		case *ast.InlineFragment:
			fragmentFields := firstLevelSelectedFields(fragments, selection.SelectionSet)
			firstLevelSelectionSet = append(firstLevelSelectionSet, fragmentFields...)
		case *ast.FragmentSpread:
			fragment := fragments.ForName(selection.Name)
			fragmentFields := firstLevelSelectedFields(fragments, fragment.SelectionSet)
			firstLevelSelectionSet = append(firstLevelSelectionSet, fragmentFields...)
		default:
			panic(fmt.Sprintf("unhandled selection type: %T", selection))
		}
	}
	return firstLevelSelectionSet
}
