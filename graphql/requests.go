package graphql

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/vektah/gqlparser/ast"
)

// RemoteSchema encapsulates a particular schema that can be executed by sending network requests to the
// specified URL.
type RemoteSchema struct {
	Schema *ast.Schema
	URL    string
}

// JSONObject is a typdef for map[string]interface{} to make structuring json responses easier.
type JSONObject map[string]interface{}

type QueryVariables map[string]interface{}

// Queryer is a interface for objects that can perform
type Queryer interface {
	Query(query string, variables QueryVariables, operationName string) (map[string]interface{}, error)
}

// MockQueryer responds with pre-defined known values when executing a query
type MockQueryer struct {
	Value map[string]interface{}
}

// Query looks up the name of the query in the map of responses and returns the value
func (q *MockQueryer) Query(query string, variables QueryVariables, operationName string) (map[string]interface{}, error) {
	return q.Value, nil
}

// NetworkQueryer sends the query to a url and returns the response
type NetworkQueryer struct {
	URL    string
	Client *http.Client
}

// Query sends the query to the designated url and returns the response.
func (q *NetworkQueryer) Query(query string, variables QueryVariables, operationName string) (map[string]interface{}, error) {
	// the payload
	payload, err := json.Marshal(JSONObject{
		"query":         query,
		"variables":     variables,
		"operationName": operationName,
	})
	if err != nil {
		return nil, err
	}

	// fire the response to the queryer's url
	resp, err := q.Client.Post(q.URL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	// read the full body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// parse the response as json
	response := JSONObject{}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	// pass the result along
	return response, nil
}

// NewNetworkQueryer returns a NetworkQueryer pointed to the given url
func NewNetworkQueryer(url string) *NetworkQueryer {
	return &NetworkQueryer{
		URL:    url,
		Client: &http.Client{},
	}
}
