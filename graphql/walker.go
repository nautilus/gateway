package graphql

import "github.com/vektah/gqlparser/ast"

// SelectionWalker is a visitor-like interface for structs that can perform a
// particular function at each selection in the tree of nested selections
// of a selection set.
type SelectionWalker interface {
	OnField(*ast.Field)
	OnInlineFragment(*ast.InlineFragment)
	OnFragmentSpread(*ast.FragmentSpread)
}

// WalkSelection traverses the provided selection set and invokes the appropriate
// methods on the walker.
func WalkSelection(walker SelectionWalker, set ast.SelectionSet) {
	// for each selection in the set
	for _, selection := range set {
		switch selection := selection.(type) {
		// invoke the appropriate handler
		case *ast.Field:
			walker.OnField(selection)
		case *ast.InlineFragment:
			walker.OnInlineFragment(selection)
		case *ast.FragmentSpread:
			walker.OnFragmentSpread(selection)
		}
	}
}
