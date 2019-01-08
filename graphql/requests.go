package graphql

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"

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

type QueryInput struct {
	Query         string
	OperationName string
	Variables     map[string]interface{}
}

// Queryer is a interface for objects that can perform
type Queryer interface {
	Query(*QueryInput, interface{}) error
}

// MockQueryer responds with pre-defined known values when executing a query
type MockQueryer struct {
	Value map[string]interface{}
}

// Query looks up the name of the query in the map of responses and returns the value
func (q *MockQueryer) Query(input *QueryInput, receiver interface{}) error {
	reflect.ValueOf(receiver).Elem().Set(reflect.ValueOf(q.Value))
	return nil
}

// NetworkQueryer sends the query to a url and returns the response
type NetworkQueryer struct {
	URL    string
	Client *http.Client
}

// Query sends the query to the designated url and returns the response.
func (q *NetworkQueryer) Query(input *QueryInput, receiver interface{}) error {
	// the payload
	payload, err := json.Marshal(JSONObject{
		"query":         input.Query,
		"variables":     input.Variables,
		"operationName": input.OperationName,
	})
	if err != nil {
		return err
	}

	// fire the response to the queryer's url
	resp, err := q.Client.Post(q.URL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	// read the full body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	err = json.Unmarshal(body, receiver)
	if err != nil {
		return err
	}

	// pass the result along
	return nil
}

// NewNetworkQueryer returns a NetworkQueryer pointed to the given url
func NewNetworkQueryer(url string) *NetworkQueryer {
	return &NetworkQueryer{
		URL:    url,
		Client: &http.Client{},
	}
}
