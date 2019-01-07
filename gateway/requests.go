package gateway

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/vektah/gqlparser/ast"
)

// Queryer is a interface for objects that can perform
type Queryer interface {
	Query(*ast.QueryDocument) (map[string]interface{}, error)
}

// MockQueryer responds with pre-defined known values when executing a query
type MockQueryer struct {
	Value JSONObject
}

// Query looks up the name of the query in the map of responses and returns the value
func (q *MockQueryer) Query(query *ast.QueryDocument) (map[string]interface{}, error) {
	return q.Value, nil
}

// NetworkQueryer sends the query to a url and returns the response
type NetworkQueryer struct {
	URL    string
	Client *http.Client
}

// Query sends the query to the designated url and returns the response.
func (q *NetworkQueryer) Query(query *ast.QueryDocument) (map[string]interface{}, error) {
	// grab the operation
	operation := query.Operations[0]

	// turn the query into a string
	queryStr, err := PrintQuery(operation)
	if err != nil {
		return nil, err
	}

	// the payload
	payload, err := json.Marshal(JSONObject{
		"operationName": operation.Name,
		"query":         queryStr,
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
	response := &JSONObject{}

	err = json.Unmarshal(body, response)
	if err != nil {
		return nil, err
	}

	// pass the result along
	return *response, nil
}

// NewNetworkQueryer returns a NetworkQueryer pointed to the given url
func NewNetworkQueryer(url string) *NetworkQueryer {
	return &NetworkQueryer{
		URL:    url,
		Client: &http.Client{},
	}
}
