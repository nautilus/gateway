package gateway

import "github.com/vektah/gqlparser/ast"

// Queryer is a interface for objects that can perform
type Queryer interface {
	Query(*ast.QueryDocument) (map[interface{}]interface{}, error)
}

// MockQueryer responds with pre-defined known values when executing a query
type MockQueryer struct {
	Value map[interface{}]interface{}
}

// Query looks up the name of the query in the map of responses and returns the value
func (q *MockQueryer) Query(query *ast.QueryDocument) (map[interface{}]interface{}, error) {
	return map[interface{}]interface{}{}, nil
}

// NetworkQueryer sends the query to a url and returns the response
type NetworkQueryer struct {
	URL string
}

// Query sends the query to the designated url and returns the response.
func (q *NetworkQueryer) Query(query *ast.QueryDocument) (map[interface{}]interface{}, error) {
	return map[interface{}]interface{}{}, nil
}
