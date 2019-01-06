package gateway

import (
	"testing"

	"github.com/vektah/gqlparser/ast"
)

func TestNetworkQueryer_sendsQueries(t *testing.T) {
	// build a query to test should be equivalent to
	// targetQueryBody := `
	// 	{
	// 		hello {
	// 			world
	// 		}
	// 	}
	// `
	// the corresponding query document
	query := &ast.QueryDocument{
		Operations: ast.OperationList{
			{
				Operation: ast.Query,
				SelectionSet: ast.SelectionSet{
					&ast.Field{
						Name: "hello",
						SelectionSet: ast.SelectionSet{
							&ast.Field{
								Name: "world",
							},
						},
					},
				},
			},
		},
	}

	queryer := &NetworkQueryer{
		URL: "hello",
	}

	// get the response of the query
	result, err := queryer.Query(query)
	if err != nil {
		t.Error(err)
		return
	}

	if result == nil {
		t.Error("Did not get a result back")
		return
	}
}
