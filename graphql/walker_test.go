package graphql

import (
	"testing"

	"github.com/vektah/gqlparser/ast"
)

func TestWalker_visitsEachSelection(t *testing.T) {
	walker := &walkerTest{}
	// a selection set to walk
	WalkSelection(walker, ast.SelectionSet{
		&ast.Field{},
		&ast.InlineFragment{},
		&ast.FragmentSpread{},
	})
}

type walkerTest struct {
	CalledField          int
	CalledInlineFragment int
	CalledFragmentSpread int
	index                int
}

func (t *walkerTest) OnField(*ast.Field) {
	t.index++
	t.CalledField = t.index
}

func (t *walkerTest) OnInlineFragment(*ast.InlineFragment) {
	t.index++
	t.CalledInlineFragment = t.index
}

func (t *walkerTest) OnFragmentSpread(*ast.FragmentSpread) {
	t.index++
	t.CalledFragmentSpread = t.index
}
