package gateway

import (
	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	"github.com/vektah/gqlparser/parser"
)

func loadSchema(typedef string) (*ast.Schema, error) {
	return gqlparser.LoadSchema(&ast.Source{
		Input: typedef,
	})
}

func parseQuery(query string) (*ast.QueryDocument, *gqlerror.Error) {
	return parser.ParseQuery(&ast.Source{Input: query})
}
