package gateway

import (
	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"
)

func loadSchema(typedef string) (*ast.Schema, error) {
	return gqlparser.LoadSchema(&ast.Source{
		Input: typedef,
	})
}
