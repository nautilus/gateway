package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

type roundTripFunc func(req *http.Request) *http.Response

// RoundTrip .
func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestNetworkQueryer_sendsQueries(t *testing.T) {
	// build a query to test should be equivalent to
	// targetQueryBody := `
	// 	{
	// 		hello(world: "hello") {
	// 			world
	// 		}
	// 	}
	// `

	// the result we expect back
	expected := map[string]interface{}{
		"data": map[string]interface{}{
			"foo": "bar",
		},
	}

	// create a http client that responds with a known body and verifies the incoming query
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			// serialize the json we want to send back
			result, err := json.Marshal(expected)
			// if something went wrong
			if err != nil {
				return &http.Response{
					StatusCode: 500,
					Body:       ioutil.NopCloser(bytes.NewBufferString("Something went wrong")),
					Header:     make(http.Header),
				}
			}

			return &http.Response{
				StatusCode: 200,
				// Send response to be tested
				Body: ioutil.NopCloser(bytes.NewBuffer(result)),
				// Must be set to non-nil value or it panics
				Header: make(http.Header),
			}
		}),
	}

	// the corresponding query document
	query := `
		{
			hello(world: "hello") {
				world
			}
		}
	`

	queryer := &NetworkQueryer{
		URL:    "hello",
		Client: client,
	}

	// get the response of the query
	result := map[string]interface{}{}
	err := queryer.Query(context.Background(), &QueryInput{Query: query}, &result)
	if err != nil {
		t.Error(err)
		return
	}
	if result == nil {
		t.Error("Did not get a result back")
		return
	}

	// make sure we got what we expected
	assert.Equal(t, expected["data"], result)
}

func TestNetworkQueryer_handlesErrorResponse(t *testing.T) {
	// the table for the tests
	for _, row := range []struct {
		Message    string
		ErrorShape interface{}
	}{
		{
			"Well Structured Error",
			[]map[string]interface{}{
				{
					"message": "message",
				},
			},
		},
		{
			"Errors Not Lists",
			map[string]interface{}{
				"message": "message",
			},
		},
		{
			"Errors Lists of Not Strings",
			[]string{"hello"},
		},
		{
			"Errors No messages",
			[]map[string]interface{}{},
		},
		{
			"Message not string",
			[]map[string]interface{}{
				{
					"message": true,
				},
			},
		},
		{
			"No Errors",
			nil,
		},
	} {
		t.Run(row.Message, func(t *testing.T) {

			// create a http client that responds with a known body and verifies the incoming query
			client := &http.Client{
				Transport: roundTripFunc(func(req *http.Request) *http.Response {
					response := map[string]interface{}{
						"data": nil,
					}

					// if we are supposed to have an error
					if row.ErrorShape != nil {
						response["errors"] = row.ErrorShape
					}

					// serialize the json we want to send back
					result, err := json.Marshal(response)
					// if something went wrong
					if err != nil {
						return &http.Response{
							StatusCode: 500,
							Body:       ioutil.NopCloser(bytes.NewBufferString("Something went wrong")),
							Header:     make(http.Header),
						}
					}

					return &http.Response{
						StatusCode: 200,
						// Send response to be tested
						Body: ioutil.NopCloser(bytes.NewBuffer(result)),
						// Must be set to non-nil value or it panics
						Header: make(http.Header),
					}
				}),
			}

			// the corresponding query document
			query := `
				{
					hello(world: "hello") {
						world
					}
				}
			`

			queryer := &NetworkQueryer{
				URL:    "hello",
				Client: client,
			}

			// get the response of the query
			result := map[string]interface{}{}
			err := queryer.Query(context.Background(), &QueryInput{Query: query}, &result)

			// if we're supposed to hav ean error
			if row.ErrorShape != nil {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestNetworkQueryer_respondsWithErr(t *testing.T) {
	// create a http client that responds with a known body and verifies the incoming query
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			// send an error back
			return &http.Response{
				StatusCode: 500,
				Body:       ioutil.NopCloser(bytes.NewBufferString("Something went wrong")),
				Header:     make(http.Header),
			}
		}),
	}

	// the corresponding query document
	query := `
		{
			hello
		}
	`

	queryer := &NetworkQueryer{
		URL:    "hello",
		Client: client,
	}

	// get the response of the query
	var result interface{}
	err := queryer.Query(context.Background(), &QueryInput{Query: query}, result)
	if err == nil {
		t.Error("Did not receive an error")
		return
	}
}

func TestNewNetworkQueryer(t *testing.T) {
	// make sure that create a new query renderer saves the right URL
	assert.Equal(t, "foo", NewNetworkQueryer("foo").URL)
}

func TestSerializeError(t *testing.T) {
	// marshal the 2 kinds of errors
	errWithCode, _ := json.Marshal(NewError("ERROR_CODE", "foo"))
	expected, _ := json.Marshal(map[string]interface{}{
		"extensions": map[string]interface{}{
			"code": "ERROR_CODE",
		},
		"message": "foo",
	})

	assert.Equal(t, string(expected), string(errWithCode))
}

func TestNetworkQueryer_errorList(t *testing.T) {
	// create a http client that responds with a known body and verifies the incoming query
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				// Send response to be tested
				Body: ioutil.NopCloser(bytes.NewBuffer([]byte(`{
					"data": null,
					"errors": [
						{"message":"hello"}
					]
				}`))),
				// Must be set to non-nil value or it panics
				Header: make(http.Header),
			}
		}),
	}

	// the corresponding query document
	query := `
		{
			hello(world: "hello") {
				world
			}
		}
	`

	queryer := &NetworkQueryer{
		URL:    "hello",
		Client: client,
	}

	// get the error of the query
	err := queryer.Query(context.Background(), &QueryInput{Query: query}, &map[string]interface{}{})

	_, ok := err.(ErrorList)
	if !ok {
		t.Errorf("response of queryer was not an error list: %v", err.Error())
		return
	}
}

func TestQueryerFunc_success(t *testing.T) {
	expected := map[string]interface{}{"hello": "world"}

	queryer := QueryerFunc(
		func(*QueryInput) (interface{}, error) {
			return expected, nil
		},
	)

	// a place to write the result
	result := map[string]interface{}{}

	err := queryer.Query(context.Background(), &QueryInput{}, &result)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure we copied the right result
	assert.Equal(t, expected, result)
}

func TestQueryerFunc_failure(t *testing.T) {
	expected := errors.New("message")

	queryer := QueryerFunc(
		func(*QueryInput) (interface{}, error) {
			return nil, expected
		},
	)

	err := queryer.Query(context.Background(), &QueryInput{}, &map[string]interface{}{})

	// make sure we got the right error
	assert.Equal(t, expected, err)
}

func TestNetworkQueryer_middlewaresFailure(t *testing.T) {
	queryer := (&NetworkQueryer{
		URL: "Hello",
	}).WithMiddlewares([]NetworkMiddleware{
		func(r *http.Request) (*http.Request, error) {
			return nil, errors.New("This One")
		},
	})

	// the input to the query
	input := &QueryInput{
		Query: "",
	}

	// fire the query
	err := queryer.Query(context.Background(), input, &map[string]interface{}{})
	if err == nil {
		t.Error("Did not enounter an error when we should have")
		return
	}
	if err.Error() != "This One" {
		t.Errorf("Did not encountered expected error message: Expected 'This One', found %v", err.Error())
	}
}
func TestNetworkQueryer_middlewaresSuccess(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			// if we did not get the right header value
			if req.Header.Get("Hello") != "World" {
				return &http.Response{
					StatusCode: http.StatusExpectationFailed,
					// Send response to be tested
					Body: ioutil.NopCloser(bytes.NewBufferString("Did not recieve the right header")),
					// Must be set to non-nil value or it panics
					Header: make(http.Header),
				}
			}

			// serialize the json we want to send back
			result, _ := json.Marshal(map[string]interface{}{
				"allUsers": []string{
					"John Jacob",
					"Jinglehymer Schmidt",
				},
			})

			return &http.Response{
				StatusCode: 200,
				// Send response to be tested
				Body: ioutil.NopCloser(bytes.NewBuffer(result)),
				// Must be set to non-nil value or it panics
				Header: make(http.Header),
			}
		}),
	}

	queryer := (&NetworkQueryer{
		URL:    "Hello",
		Client: httpClient,
	}).WithMiddlewares([]NetworkMiddleware{
		func(r *http.Request) (*http.Request, error) {
			r.Header.Set("Hello", "World")

			return r, nil
		},
	})

	// the input to the query
	input := &QueryInput{
		Query: "",
	}

	err := queryer.Query(context.Background(), input, &map[string]interface{}{})
	if err != nil {
		t.Error(err.Error())
		return
	}
}
