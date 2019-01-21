package graphql

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/vektah/gqlparser/ast"
)

// RemoteSchema encapsulates a particular schema that can be executed by sending network requests to the
// specified URL.
type RemoteSchema struct {
	Schema *ast.Schema
	URL    string
}

// QueryInput provides all of the information required to fire a query
type QueryInput struct {
	Query         string
	QueryDocument *ast.QueryDocument
	OperationName string
	Variables     map[string]interface{}
}

// Queryer is a interface for objects that can perform
type Queryer interface {
	Query(*QueryInput, interface{}) error
}

// MockSuccessQueryer responds with pre-defined value when executing a query
type MockSuccessQueryer struct {
	Value interface{}
}

// Query looks up the name of the query in the map of responses and returns the value
func (q *MockSuccessQueryer) Query(input *QueryInput, receiver interface{}) error {
	// assume the mock is writing the same kind as the receiver
	reflect.ValueOf(receiver).Elem().Set(reflect.ValueOf(q.Value))

	// this will panic if something goes wrong
	return nil
}

// QueryerFunc responds to the query by calling the provided function
type QueryerFunc struct {
	Fn func(*QueryInput) (interface{}, error)
}

// Query invokes the provided function and writes the response to the receiver
func (q *QueryerFunc) Query(input *QueryInput, receiver interface{}) error {
	// invoke the handler
	response, err := q.Fn(input)
	if err != nil {
		return err
	}

	// assume the mock is writing the same kind as the receiver
	reflect.ValueOf(receiver).Elem().Set(reflect.ValueOf(response))

	// no errors
	return nil
}

// NetworkQueryer sends the query to a url and returns the response
type NetworkQueryer struct {
	URL    string
	Client *http.Client
}

// IntrospectRemoteSchema is used to build a RemoteSchema by firing the introspection query
// at a remote service and reconstructing the schema object from the response
func IntrospectRemoteSchema(url string) (*RemoteSchema, error) {
	// introspect the schema at the designated url
	schema, err := IntrospectAPI(NewNetworkQueryer(url))
	if err != nil {
		return nil, err
	}

	return &RemoteSchema{
		URL:    url,
		Schema: schema,
	}, nil
}

// Query sends the query to the designated url and returns the response.
func (q *NetworkQueryer) Query(input *QueryInput, receiver interface{}) error {
	// the payload
	payload, err := json.Marshal(map[string]interface{}{
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

	result := map[string]interface{}{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return err
	}

	// if there is an error
	if _, ok := result["errors"]; ok {
		// a list of errors from the response
		errList := ErrorList{}

		// build up a list of errors
		errs, ok := result["errors"].([]interface{})
		if !ok {
			return errors.New("errors was not a list")
		}

		// a list of error messages
		for _, err := range errs {
			obj, ok := err.(map[string]interface{})
			if !ok {
				return errors.New("encountered non-object error")
			}

			message, ok := obj["message"].(string)
			if !ok {
				return errors.New("error message was not a string")
			}

			errList = append(errList, NewError("", message))
		}

		return errList
	}

	// assign the result under the data key to the receiver
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "json",
		Result:  receiver,
	})
	if err != nil {
		return err
	}

	err = decoder.Decode(result["data"])
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

// ErrorExtensions define fields that extend the standard graphql error shape
type ErrorExtensions struct {
	Code string `json:"code"`
}

// Error represents a graphql error
type Error struct {
	Extensions ErrorExtensions `json:"extensions"`
	Message    string          `json:"message"`
}

func (e *Error) Error() string {
	return e.Message
}

// NewError returns a graphql error with the given code and message
func NewError(code string, message string) *Error {
	return &Error{
		Message: message,
		Extensions: ErrorExtensions{
			Code: code,
		},
	}
}

// ErrorList represents a list of errors
type ErrorList []error

// Error returns a string representation of each error
func (list ErrorList) Error() string {
	acc := []string{}

	for _, error := range list {
		acc = append(acc, error.Error())
	}

	return strings.Join(acc, ". ")
}
