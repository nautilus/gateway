package graphql

import (
	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"
)

// LoadSchema takes an SDL string and returns the parsed version
func LoadSchema(typedef string) (*ast.Schema, error) {
	return gqlparser.LoadSchema(&ast.Source{
		Input: typedef,
	})
}
