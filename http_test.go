package gateway

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alecaivazis/graphql-gateway/graphql"
	"github.com/stretchr/testify/assert"
)

func TestHttpHandler_postMissingQuery(t *testing.T) {
	schema, err := graphql.LoadSchema(`
		type Query {
			allUsers: [String!]!
		}
	`)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url1"},
	})
	if err != nil {
		t.Error(err.Error())
		return
	}
	// the incoming request
	request := httptest.NewRequest("POST", "/graphql", strings.NewReader(`
		{
			"query": ""
		}
	`))
	// a recorder so we can check what the handler responded with
	responseRecorder := httptest.NewRecorder()

	// call the http hander
	gateway.GraphQLHandler(responseRecorder, request)

	// make sure we got an error code
	assert.Equal(t, http.StatusUnprocessableEntity, responseRecorder.Result().StatusCode)
}

func TestHttpHandler_getMissingQuery(t *testing.T) {
	schema, _ := graphql.LoadSchema(`
		type Query {
			allUsers: [String!]!
		}
	`)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url1"},
	})
	if err != nil {
		t.Error(err.Error())
		return
	}
	// the incoming request
	request := httptest.NewRequest("GET", "/graphql", strings.NewReader(""))
	// a recorder so we can check what the handler responded with
	responseRecorder := httptest.NewRecorder()

	// call the http hander
	gateway.GraphQLHandler(responseRecorder, request)

	// make sure we got an error code
	assert.Equal(t, http.StatusUnprocessableEntity, responseRecorder.Result().StatusCode)
}

func TestHttpHandler_executionError(t *testing.T) {
	// a planner that always returns an error
	planner := &MockErrPlanner{Err: errors.New("Planning error")}

	// and some schemas that the gateway wraps
	schema, err := graphql.LoadSchema(`
		type Query {
			allUsers: [String!]!
		}
	`)
	schemas := []*graphql.RemoteSchema{{Schema: schema, URL: "url1"}}

	// create gateway schema we can test against
	gateway, err := New(schemas, WithPlanner(planner))
	if err != nil {
		t.Error(err.Error())
		return
	}
	// the incoming request
	request := httptest.NewRequest("POST", "/graphql", strings.NewReader(`
		{
			"query": "{ allUsers }"
		}
	`))
	// a recorder so we can check what the handler responded with
	responseRecorder := httptest.NewRecorder()

	// call the http hander
	gateway.GraphQLHandler(responseRecorder, request)

	// get the response from the handler
	response := responseRecorder.Result()
	// read the body
	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure we got an error code
	assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
	// and the right body
	assert.Contains(t, string(responseBody), "Planning error")
}
