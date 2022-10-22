package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/nautilus/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/v2/ast"
)

type resultWithErrors struct {
	Errors []struct {
		Extensions map[string]string `json:"extensions"`
		Message    string            `json:"message"`
	} `json:"errors"`
}

func TestGraphQLHandler_postMissingQuery(t *testing.T) {
	t.Parallel()
	schema, err := graphql.LoadSchema(`
		type Query {
			allUsers: [String!]!
		}
	`)
	assert.NoError(t, err)

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
	result := responseRecorder.Result()
	assert.NoError(t, result.Body.Close())
	assert.Equal(t, http.StatusUnprocessableEntity, result.StatusCode)
}

func TestGraphQLHandler(t *testing.T) {
	t.Parallel()
	schema, _ := graphql.LoadSchema(`
		type Query {
			allUsers: [String!]!
		}
	`)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url1"},
	}, WithExecutor(ExecutorFunc(
		func(*ExecutionContext) (map[string]interface{}, error) {
			return map[string]interface{}{
				"Hello": "world",
			}, nil
		},
	)))

	if err != nil {
		t.Error(err.Error())
		return
	}

	t.Run("Missing query", func(t *testing.T) {
		t.Parallel()
		// the incoming request
		request := httptest.NewRequest("GET", "/graphql", strings.NewReader(""))
		// a recorder so we can check what the handler responded with
		responseRecorder := httptest.NewRecorder()

		// call the http hander
		gateway.GraphQLHandler(responseRecorder, request)

		// make sure we got an error code
		recorderResult := responseRecorder.Result()
		assert.NoError(t, recorderResult.Body.Close())
		assert.Equal(t, http.StatusUnprocessableEntity, recorderResult.StatusCode)

		// verify the graphql error code
		result, err := readResultWithErrors(responseRecorder, t)
		if err != nil {
			assert.Error(t, err)
		}
		assert.Equal(t, result.Errors[0].Extensions["code"], "BAD_USER_INPUT")
	})

	t.Run("Non-object variables fails", func(t *testing.T) {
		t.Parallel()
		// the incoming request
		request := httptest.NewRequest("GET", `/graphql?query={allUsers}&variables=true`, strings.NewReader(""))

		// a recorder so we can check what the handler responded with
		responseRecorder := httptest.NewRecorder()
		// call the http hander
		gateway.GraphQLHandler(responseRecorder, request)

		// make sure we got an error code
		result := responseRecorder.Result()
		assert.NoError(t, result.Body.Close())
		assert.Equal(t, http.StatusUnprocessableEntity, result.StatusCode)
	})

	t.Run("Object variables succeeds", func(t *testing.T) {
		t.Parallel()
		// the incoming request
		request := httptest.NewRequest("GET", `/graphql?query={allUsers}&variables={"foo":2}`, strings.NewReader(""))
		// a recorder so we can check what the handler responded with
		responseRecorder := httptest.NewRecorder()

		// call the http hander
		gateway.GraphQLHandler(responseRecorder, request)

		// make sure we got an error code
		result := responseRecorder.Result()
		assert.NoError(t, result.Body.Close())
		assert.Equal(t, http.StatusOK, result.StatusCode)
	})

	t.Run("OperationName", func(t *testing.T) {
		t.Parallel()
		// the incoming request
		request := httptest.NewRequest("GET", `/graphql?query={allusers}&operationName=Hello`, strings.NewReader(""))
		// a recorder so we can check what the handler responded with
		responseRecorder := httptest.NewRecorder()

		// call the http hander
		gateway.GraphQLHandler(responseRecorder, request)

		// make sure we got an error code
		result := responseRecorder.Result()
		assert.NoError(t, result.Body.Close())
		assert.Equal(t, http.StatusBadRequest, result.StatusCode)
	})

	t.Run("error marhsalling response", func(t *testing.T) {
		t.Parallel()
		// create gateway schema we can test against
		innerGateway, err := New([]*graphql.RemoteSchema{
			{Schema: schema, URL: "url1"},
		}, WithExecutor(ExecutorFunc(
			func(*ExecutionContext) (map[string]interface{}, error) {
				return map[string]interface{}{
					"foo": func() {},
				}, nil
			},
		)))

		if err != nil {
			t.Error(err.Error())
			return
		}

		// the incoming request
		request := httptest.NewRequest("GET", `/graphql?query={allUsers}`, strings.NewReader(""))
		// a recorder so we can check what the handler responded with
		responseRecorder := httptest.NewRecorder()

		// call the http hander
		innerGateway.GraphQLHandler(responseRecorder, request)

		// make sure we got an error code
		recorderResult := responseRecorder.Result()
		assert.NoError(t, recorderResult.Body.Close())
		assert.Equal(t, http.StatusInternalServerError, recorderResult.StatusCode)

		// verify the graphql error code
		result, err := readResultWithErrors(responseRecorder, t)
		if err != nil {
			assert.Error(t, err)
		}
		assert.Equal(t, result.Errors[0].Extensions["code"], "UNKNOWN_ERROR")
	})

	t.Run("internal server error response", func(t *testing.T) {
		t.Parallel()
		// create gateway schema we can test against
		innerGateway, err := New([]*graphql.RemoteSchema{
			{Schema: schema, URL: "url1"},
		}, WithExecutor(ExecutorFunc(
			func(*ExecutionContext) (map[string]interface{}, error) {
				return nil, errors.New("error string")
			},
		)))

		if err != nil {
			t.Error(err.Error())
			return
		}

		// the incoming request
		request := httptest.NewRequest("GET", `/graphql?query={allUsers}`, strings.NewReader(""))
		// a recorder so we can check what the handler responded with
		responseRecorder := httptest.NewRecorder()

		// call the http hander
		innerGateway.GraphQLHandler(responseRecorder, request)

		// make sure we got an error code
		recorderResult := responseRecorder.Result()
		assert.NoError(t, recorderResult.Body.Close())
		assert.Equal(t, http.StatusOK, recorderResult.StatusCode)

		// verify the graphql error code
		result, err := readResultWithErrors(responseRecorder, t)
		if err != nil {
			assert.Error(t, err)
		}
		assert.Equal(t, result.Errors[0].Extensions["code"], "INTERNAL_SERVER_ERROR")
	})
}

func readResultWithErrors(responseRecorder *httptest.ResponseRecorder, t *testing.T) (*resultWithErrors, error) {
	t.Helper()
	recorderResult := responseRecorder.Result()
	defer recorderResult.Body.Close()
	body, err := io.ReadAll(recorderResult.Body)
	if err != nil {
		return nil, err
	}

	result := resultWithErrors{}
	err = json.Unmarshal(body, &result)
	return &result, err
}

func TestQueryPlanCacheParameters_post(t *testing.T) {
	t.Parallel()
	// load the schema we'll test
	schema, _ := graphql.LoadSchema(`
		type Query {
			allUsers: [String!]!
		}
	`)

	// the expected result
	expectedResult := map[string]interface{}{
		"Hello": "world",
	}

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url1"},
	}, WithExecutor(ExecutorFunc(
		func(*ExecutionContext) (map[string]interface{}, error) {
			return expectedResult, nil
		},
	)), WithAutomaticQueryPlanCache())
	if err != nil {
		t.Error(err)
		return
	}

	// make a request for an unknown persisted query
	request := httptest.NewRequest("POST", "/graphql", strings.NewReader(`
		{
			"extensions": {
				"persistedQuery": {
					"version": 1,
					"sha256Hash": "1234"
				}
			}
		}
	`))
	// a recorder so we can check what the handler responded with
	responseRecorder := httptest.NewRecorder()

	// call the http hander
	gateway.PlaygroundHandler(responseRecorder, request)

	// get the response from the handler
	response := responseRecorder.Result()

	// make sure we got a bad status
	if !assert.Equal(t, http.StatusBadRequest, response.StatusCode) {
		return
	}
	// the body of the response
	body := struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}{}
	// parse the response
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Error(err)
		return
	}

	// make sure that the response is what we expect
	if !assert.Equal(t, "PersistedQueryNotFound", body.Errors[0].Message) {
		return
	}

	// passing in a valid query along with the hash
	request = httptest.NewRequest("POST", "/graphql", strings.NewReader(`
		{
			"query": "{ allUsers }",
			"extensions": {
				"persistedQuery": {
					"version": 1,
					"sha256Hash": "1234"
				}
			}
		}
	`))
	// a recorder so we can check what the handler responded with
	responseRecorder2 := httptest.NewRecorder()

	// call the http hander
	gateway.PlaygroundHandler(responseRecorder2, request)
	// get the response from the handler
	response2 := responseRecorder2.Result()

	// make sure we got an OK status
	if !assert.Equal(t, http.StatusOK, response2.StatusCode) {
		return
	}

	// and the expected result
	result := map[string]interface{}{}
	defer response2.Body.Close()
	if err := json.NewDecoder(response2.Body).Decode(&result); err != nil {
		t.Error(err)
		return
	}

	// the expected result
	expected := map[string]interface{}{
		"data": expectedResult,
		"extensions": map[string]interface{}{
			"persistedQuery": map[string]interface{}{
				"sha265Hash": "1234",
				"version":    "1",
			},
		},
	}

	assert.Equal(t, expected, result)
}

func TestQueryPlanCacheParameters_get(t *testing.T) {
	t.Parallel()
	// load the schema we'll test
	schema, _ := graphql.LoadSchema(`
		type Query {
			allUsers: [String!]!
		}
	`)

	// the expected result
	expectedResult := map[string]interface{}{
		"Hello": "world",
	}

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url1"},
	}, WithExecutor(ExecutorFunc(
		func(*ExecutionContext) (map[string]interface{}, error) {
			return expectedResult, nil
		},
	)), WithAutomaticQueryPlanCache())
	if err != nil {
		t.Error(err)
		return
	}

	// make a request for an unknown persisted query
	// request := httptesot.NewRequest("POST", "/graphql?extensions={\"persistedQuery\": {\"version\": 1, \"sha256Hash\": \"1234\"}}", strings.NewReader(""))
	request := &http.Request{
		Method: "GET",
		URL: &url.URL{
			RawPath:  "/graphql",
			RawQuery: "extensions={\"persistedQuery\": {\"version\": 1, \"sha256Hash\": \"1234\"}}",
		},
	}
	// a recorder so we can check what the handler responded with
	responseRecorder := httptest.NewRecorder()

	// call the http hander
	gateway.GraphQLHandler(responseRecorder, request)

	// get the response from the handler
	response := responseRecorder.Result()

	// make sure we got a bad status
	if !assert.Equal(t, http.StatusBadRequest, response.StatusCode) {
		return
	}
	// the body of the response
	body := struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}{}
	// parse the response
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Error(err)
		return
	}

	// make sure that the response is what we expect
	if !assert.Equal(t, "PersistedQueryNotFound", body.Errors[0].Message) {
		return
	}
}

func TestPlaygroundHandler_postRequest(t *testing.T) {
	t.Parallel()
	// a planner that always returns an error
	planner := &MockErrPlanner{Err: errors.New("Planning error")}

	// and some schemas that the gateway wraps
	schema, err := graphql.LoadSchema(`
		type Query {
			allUsers: [String!]!
		}
	`)
	assert.NoError(t, err)
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
	gateway.PlaygroundHandler(responseRecorder, request)

	// get the response from the handler
	response := responseRecorder.Result()
	defer response.Body.Close()
	// read the body
	_, err = io.ReadAll(response.Body)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// make sure we got an error code
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestPlaygroundHandler_postRequestList(t *testing.T) {
	t.Parallel()
	// and some schemas that the gateway wraps
	schema, err := graphql.LoadSchema(`
		type User {
			id: ID!
		}
	`)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// some fields to query
	aField := &QueryField{
		Name: "a",
		Type: ast.NamedType("User", &ast.Position{}),
		Resolver: func(ctx context.Context, arguments map[string]interface{}) (string, error) {
			return "a", nil
		},
	}
	bField := &QueryField{
		Name: "b",
		Type: ast.NamedType("User", &ast.Position{}),
		Resolver: func(ctx context.Context, arguments map[string]interface{}) (string, error) {
			return "b", nil
		},
	}

	// instantiate the gateway
	gw, err := New([]*graphql.RemoteSchema{{URL: "url1", Schema: schema}}, WithQueryFields(aField, bField))
	if err != nil {
		t.Error(err.Error())
		return
	}

	// we need to send a list of two queries ({ a } and { b }) and make sure they resolve in the right order

	// the incoming request
	request := httptest.NewRequest("POST", "/graphql", strings.NewReader(`
		[
			{
				"query": "{ a { id } }"
			},
			{
				"query": "{ b { id } }"
			}
		]
	`))
	// a recorder so we can check what the handler responded with
	responseRecorder := httptest.NewRecorder()

	// call the http hander
	gw.PlaygroundHandler(responseRecorder, request)
	// get the response from the handler
	response := responseRecorder.Result()
	defer response.Body.Close()

	// make sure we got a successful response
	if !assert.Equal(t, http.StatusOK, response.StatusCode) {
		return
	}

	// read the body
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Error(err.Error())
		return
	}

	result := []map[string]interface{}{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		t.Error(err.Error())
		return
	}

	// we should have gotten 2 responses
	if !assert.Len(t, result, 2) {
		return
	}

	// make sure there were no errors in the first query
	if firstQuery := result[0]; assert.Nil(t, firstQuery["errors"]) {
		// make sure it has the right id
		assert.Equal(t, map[string]interface{}{"a": map[string]interface{}{"id": "a"}}, firstQuery["data"])
	}

	// make sure there were no errors in the second query
	if secondQuery := result[1]; assert.Nil(t, secondQuery["errors"]) {
		// make sure it has the right id
		assert.Equal(t, map[string]interface{}{"b": map[string]interface{}{"id": "b"}}, secondQuery["data"])
	}
}

func TestPlaygroundHandler_getRequest(t *testing.T) {
	t.Parallel()
	// a planner that always returns an error
	planner := &MockErrPlanner{Err: errors.New("Planning error")}

	// and some schemas that the gateway wraps
	schema, err := graphql.LoadSchema(`
		type Query {
			allUsers: [String!]!
		}
	`)
	assert.NoError(t, err)
	schemas := []*graphql.RemoteSchema{{Schema: schema, URL: "url1"}}

	// create gateway schema we can test against
	gateway, err := New(schemas, WithPlanner(planner))
	if err != nil {
		t.Error(err.Error())
		return
	}
	// the incoming request
	request := httptest.NewRequest("GET", "/graphql", strings.NewReader(``))
	// a recorder so we can check what the handler responded with
	responseRecorder := httptest.NewRecorder()

	// call the http hander
	gateway.PlaygroundHandler(responseRecorder, request)

	result := responseRecorder.Result()
	_, err = html.Parse(result.Body)
	defer result.Body.Close()

	if err != nil {
		t.Error(err.Error())
		return
	}
}

func TestGraphQLHandler_postWithFile(t *testing.T) {
	t.Parallel()
	schema, err := graphql.LoadSchema(`
		scalar Upload

		input FileInput {
			file: Upload!
		}

		type Query {
			file(id: String!): String
		}

		type Mutation {
			upload(file: Upload!): String!
			uploadInput(input: FileInput!): String!
		}
	`)
	assert.NoError(t, err)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url-file-upload"},
	}, WithExecutor(ExecutorFunc(
		func(*ExecutionContext) (map[string]interface{}, error) {
			return map[string]interface{}{
				"upload":      "file-id",
				"uploadInput": "file-id",
			}, nil
		},
	)))

	if err != nil {
		t.Error(err.Error())
		return
	}

	for _, queryTest := range []struct {
		mess       string
		operations string
		fileMap    string
		file       []byte
	}{
		{
			"Raw Upload Variable",
			`{ 
				"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
				"variables": { "someFile": null } 
			}`,
			`{ "0": ["variables.someFile"] }`,
			[]byte("Test file content1"),
		},
		{
			"Input Variable",
			`{ 
				"query": "mutation ($input: FileInput!) { uploadInput(input: $input) }", 
				"variables": { "input": { "file": null } } 
			}`,
			`{ "0": ["variables.input.file"] }`,
			[]byte("Test file content1"),
		},
	} {
		queryTest := queryTest // enable parallel sub-tests
		t.Run(queryTest.mess, func(t *testing.T) {
			t.Parallel()
			request, err := createMultipartRequest(
				[]byte(queryTest.operations),
				[]byte(queryTest.fileMap),
				queryTest.file,
			)

			if err != nil {
				t.Error(err)
				return
			}
			// a recorder so we can check what the handler responded with
			responseRecorder := httptest.NewRecorder()

			// call the http hander
			gateway.GraphQLHandler(responseRecorder, request)

			// make sure we got a response code (200)
			result := responseRecorder.Result()
			assert.NoError(t, result.Body.Close())
			assert.Equal(t, http.StatusOK, result.StatusCode)
		})
	}
}

func TestGraphQLHandler_DeeplyNestedFileInput(t *testing.T) {
	t.Parallel()
	schema, err := graphql.LoadSchema(`
		scalar Upload

		input WrapperOne {
			wrapperOne: WrapperTwo!
		}

		input WrapperTwo {
			wrapperTwo: FileInput!
		}

		input FileInput {
			file: Upload!
			files: [Upload!]!
		}

		type Query {
			file(id: String!): String
		}

		type Mutation {
			uploadInputWrapper(input: WrapperOne!): String!
		}
	`)
	assert.NoError(t, err)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url-file-upload"},
	}, WithExecutor(ExecutorFunc(
		func(*ExecutionContext) (map[string]interface{}, error) {
			return map[string]interface{}{
				"uploadInputWrapper": "file-id",
			}, nil
		},
	)))

	if err != nil {
		t.Error(err.Error())
		return
	}

	for _, queryTest := range []struct {
		mess       string
		operations string
		fileMap    string
		file       []byte
	}{
		{
			"Raw Upload Variable",
			`{ 
				"query": "mutation ($input: WrapperOne!) { uploadInputWrapper(input: $input) }", 
				"variables": { "input": { "wrapperOne": { "wrapperTwo": { "file": null } } } }
			}`,
			`{ "0": ["variables.input.wrapperOne.wrapperTwo.file"] }`,
			[]byte("Test file content1"),
		},
	} {
		queryTest := queryTest // enable parallel sub-tests
		t.Run(queryTest.mess, func(t *testing.T) {
			t.Parallel()
			request, err := createMultipartRequest(
				[]byte(queryTest.operations),
				[]byte(queryTest.fileMap),
				queryTest.file,
			)

			if err != nil {
				t.Error(err)
				return
			}
			// a recorder so we can check what the handler responded with
			responseRecorder := httptest.NewRecorder()

			// call the http hander
			gateway.GraphQLHandler(responseRecorder, request)

			fmt.Println(responseRecorder.Body)

			// make sure we got a response code (200)
			result := responseRecorder.Result()
			assert.NoError(t, result.Body.Close())
			assert.Equal(t, http.StatusOK, result.StatusCode)
		})
	}
}

func TestGraphQLHandler_postWithMultipleFiles(t *testing.T) {
	t.Parallel()
	schema, err := graphql.LoadSchema(`
		scalar Upload

		input FilesInput {
			files: [Upload!]!
		}

		type Query {
			file(id: String!): String
		}

		type Mutation {
			upload(file: Upload!): String!
			uploadMulti(files: [Upload!]!): [String!]!
			uploadMultiInput(input: FilesInput!): [String!]!
		}
	`)
	assert.NoError(t, err)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url-file-upload"},
	}, WithExecutor(ExecutorFunc(
		func(*ExecutionContext) (map[string]interface{}, error) {
			return map[string]interface{}{
				"upload":           "file-id1",
				"uploadMulti":      []string{"file-id2", "file-id3"},
				"uploadMultiInput": []string{"file-id1", "file-id2", "file-id3"},
			}, nil
		},
	)))

	if err != nil {
		t.Error(err.Error())
		return
	}

	for _, queryTest := range []struct {
		mess       string
		operations string
		fileMap    string
		files      [][]byte
	}{
		{
			"Multiple File Upload Raw Variable",
			`{
				"query":"mutation TestFileUpload($someFile: Upload!, $allFiles: [Upload!]!) { upload(file: $someFile) uploadMulti(files: $allFiles)}",
				"variables":{"someFile":null,"allFiles":[null,null]},"operationName":"TestFileUpload"
			}`,
			`{"0":["variables.someFile"],"1":["variables.allFiles.0"],"2":["variables.allFiles.1"]}`,
			[][]byte{
				[]byte("Test file content 1"),
				[]byte("Test file content 2"),
				[]byte("Test file content 3"),
			},
		},
		{
			"Multiple File Upload Input Variable",
			`{ 
				"query": "mutation ($input: FilesInput!) { uploadMultiInput(input: $input) }", 
				"variables": { "input": { "files": [null, null, null] } } 
			}`,
			`{"0":["variables.input.files.0"],"1":["variables.input.files.1"],"2":["variables.input.files.2"]}`,
			[][]byte{
				[]byte("Test file content 0"),
				[]byte("Test file content 1"),
				[]byte("Test file content 2"),
			},
		},
	} {
		queryTest := queryTest // enable parallel sub-tests
		t.Run(queryTest.mess, func(t *testing.T) {
			t.Parallel()
			request, err := createMultipartRequest(
				[]byte(queryTest.operations),
				[]byte(queryTest.fileMap),
				queryTest.files...,
			)

			if err != nil {
				t.Error(err)
				return
			}
			// a recorder so we can check what the handler responded with
			responseRecorder := httptest.NewRecorder()

			// call the http handler
			gateway.GraphQLHandler(responseRecorder, request)

			// make sure we got a response code (200)
			result := responseRecorder.Result()
			assert.NoError(t, result.Body.Close())
			assert.Equal(t, http.StatusOK, result.StatusCode)
		})
	}
}

func TestGraphQLHandler_postBatchWithMultipleFiles(t *testing.T) {
	t.Parallel()
	schema, err := graphql.LoadSchema(`
		scalar Upload

		input FilesInput {
			files: [Upload!]!
		}

		type Query {
			file(id: String!): String
		}

		type Mutation {
			upload(file: Upload!): String!
			uploadMulti(files: [Upload!]!): [String!]!
			uploadMultiInput(input: FilesInput!): [String!]!
		}
	`)
	assert.NoError(t, err)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url-file-upload"},
	}, WithExecutor(ExecutorFunc(
		func(*ExecutionContext) (map[string]interface{}, error) {
			return map[string]interface{}{
				"upload":           "file-id1",
				"uploadMulti":      []string{"file-id2", "file-id3"},
				"uploadMultiInput": []string{"file-id4", "file-id5", "file-id6"},
			}, nil
		},
	)))

	if err != nil {
		t.Error(err.Error())
		return
	}

	request, err := createMultipartRequest(
		[]byte(`[
			{
				"query":"mutation ($someFile: Upload!) { upload(file: $someFile) }",
				"variables":{"someFile":null}
			}, 
			{
				"query":"mutation TestFileUpload(\n $someFile: Upload!,\n\t$allFiles: [Upload!]!\n) {\n  upload(file: $someFile)\n  uploadMulti(files: $allFiles)\n}",
				"variables":{"someFile":null,"allFiles":[null,null]},"operationName":"TestFileUpload"
			},
			{ 
				"query": "mutation ($input: FilesInput!) { uploadMultiInput(input: $input) }", 
				"variables": { "input": { "files": [null, null, null] } } 
			}
		]`),
		[]byte(`{"0":["0.variables.someFile"],"1":["1.variables.someFile"],"2":["1.variables.allFiles.0"],"3":["1.variables.allFiles.1"],"4":["2.variables.input.files.0"],"5":["2.variables.input.files.1"],"6":["2.variables.input.files.2"]}`),
		[]byte("Test file content 0"),
		[]byte("Test file content 1"),
		[]byte("Test file content 2"),
		[]byte("Test file content 3"),
		[]byte("Test file content 4"),
		[]byte("Test file content 5"),
		[]byte("Test file content 6"),
	)

	if err != nil {
		t.Error(err)
		return
	}

	// a recorder so we can check what the handler responded with
	responseRecorder := httptest.NewRecorder()

	// call the http hander
	gateway.GraphQLHandler(responseRecorder, request)

	// make sure we got an error code
	result := responseRecorder.Result()
	assert.NoError(t, result.Body.Close())
	assert.Equal(t, http.StatusOK, result.StatusCode)
}

func TestGraphQLHandler_postFilesWithError(t *testing.T) {
	t.Parallel()
	schema, err := graphql.LoadSchema(`
		scalar Upload

		input FileInput {
			file: Upload!
		}

		input FilesInput {
			files: [Upload!]!
		}

		type Query {
			file(id: String!): String
		}

		type Mutation {
			upload(file: Upload!): String!
			uploadInput(input: FileInput!): String!
			uploadMulti(files: [Upload!]!): [String!]!
			uploadMultiInput(input: FilesInput!): [String!]!
		}
	`)
	assert.NoError(t, err)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url-file-upload"},
	})

	if err != nil {
		t.Error(err.Error())
		return
	}

	for _, queryTest := range []struct {
		mess       string
		operations string
		fileMap    string
		files      [][]byte
	}{
		{
			"Missing file",
			`{ 
				"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
				"variables": { "someFile": null } 
			}`,
			`{ "0": ["variables.someFile"] }`,
			[][]byte{},
		},
		{
			"Missing path variables",
			`{ 
				"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
				"variables": { "someFile": null } 
			}`,
			`{ "0": ["variables"] }`,
			[][]byte{
				[]byte("File content"),
			},
		},
		{
			"Broken operations json",
			`{"query"`,
			`{ "0": ["variables.someFile"] }`,
			[][]byte{},
		},
		{
			"Broken file map json",
			`{ 
				"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
				"variables": { "someFile": null } 
			}`,
			`{ "0"`,
			[][]byte{},
		},
		{
			"Wrong file map format - no variables",
			`{ 
				"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
				"variables": { "someFile": null } 
			}`,
			`{ "0": ["someFile"] }`,
			[][]byte{
				[]byte("File content"),
			},
		},
		{
			"Wrong file map format - invalid number of parts",
			`{ 
					"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
					"variables": { "someFile": null } 
				}`,
			`{ "0": ["variables.someFile.1.1"] }`,
			[][]byte{
				[]byte("File content"),
			},
		},
		{
			"Wrong batch index",
			`[
				{ 
					"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
					"variables": { "someFile": null } 
				},
				{ 
					"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
					"variables": { "someFile": null } 
				}
			]`,
			`{ "0": ["ololo.variables.someFile"], "1": ["1.variables.someFile"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Different files mapped to same path",
			`[
				{ 
					"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
					"variables": { "someFile": null } 
				},
				{ 
					"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
					"variables": { "someFile": null } 
				}
			]`,
			`{ "0": ["0.variables.someFile"], "1": ["0.variables.someFile"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Missing variables field for file",
			`{
				"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }",
				"variables": { "foo": "bar" }
			}`,
			`{ "0": ["variables.someFile"] }`,
			[][]byte{
				[]byte("File content 1"),
			},
		},
		{
			"Missing variables for multi upload",
			`{
				"query": "mutation ($allFiles: [Upload!]!) { uploadMulti(files: $allFiles) }",
				"variables": {}
			}`,
			`{ "0": ["variables.allFiles.0"], "1": ["variables.allFiles.1"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Wrong variable field type for multi upload",
			`{
				"query": "mutation ($allFiles: [Upload!]!) { uploadMulti(files: $allFiles) }",
				"variables": { "allFiles": 50 }
			}`,
			`{ "0": ["variables.allFiles.0"], "1": ["variables.allFiles.1"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Wrong file index in file map",
			`{
				"query": "mutation ($allFiles: [Upload!]!) { uploadMulti(files: $allFiles) }",
				"variables": { "allFiles": [null, null] }
			}`,
			`{ "0": ["variables.allFiles.foo"], "1": ["variables.allFiles.1"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Wrong file placeholders count",
			`{
				"query": "mutation ($allFiles: [Upload!]!) { uploadMulti(files: $allFiles) }",
				"variables": { "allFiles": [null] }
			}`,
			`{ "0": ["variables.allFiles.0"], "1": ["variables.allFiles.1"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Wrong file placeholder value",
			`{
				"query": "mutation ($allFiles: [Upload!]!) { uploadMulti(files: $allFiles) }",
				"variables": { "allFiles": [null, 50] }
			}`,
			`{ "0": ["variables.allFiles.0"], "1": ["variables.allFiles.1"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Missing file with input type",
			`{ 
				"query": "mutation ($input: FileInput!) { uploadInput(input: $input) }", 
				"variables": { "input": { "file": null } } 
			}`,
			`{ "0": ["variables.input.file"] }`,
			[][]byte{},
		},
		{
			"Different files mapped to same path using input type",
			`[
				{ 
					"query": "mutation ($input: FileInput!) { uploadInput(input: $input) }", 
					"variables": { "input": { "file": null } } 
				},
				{ 
					"query": "mutation ($input: FileInput!) { uploadInput(input: $input) }", 
					"variables": { "input": { "file": null } } 
				}
			]`,
			`{ "0": ["0.variables.input.file"], "1": ["0.variables.input.file"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Missing variables field for file using input type",
			`{
					"query": "mutation ($input: FileInput!) { uploadInput(input: $input) }", 
					"variables": { "input": { "foo": bar } } 
			}`,
			`{ "0": ["variables.input.file"] }`,
			[][]byte{
				[]byte("File content 1"),
			},
		},
		{
			"Missing variables for multi upload using input type",
			`{
				"query": "mutation ($input: FilesInput!) { uploadMultiInput(input: $input) }",
				"variables": {}
			}`,
			`{ "0": ["variables.input.files.0"], "1": ["variables.input.files.1"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Wrong variable field type for multi upload using input type",
			`{
				"query": "mutation ($input: FilesInput!) { uploadMultiInput(input: $input) }",
				"variables": { "input": { "files": true } }
			}`,
			`{ "0": ["variables.allFiles.0"], "1": ["variables.allFiles.1"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Wrong or missing file index in file map using input type",
			`{
				"query": "mutation ($input: FilesInput!) { uploadMultiInput(input: $input) }",
				"variables": { "input": { "files": [null, null] } }
			}`,
			`{ "0": ["variables.input.files"], "1": ["variables.input.files.one"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Wrong file placeholders count using input type",
			`{
				"query": "mutation ($input: FilesInput!) { uploadMultiInput(input: $input) }",
				"variables": { "input": { "files": [null] } }
			}`,
			`{ "0": ["variables.input.files.0"], "1": ["variables.input.files.1"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
		{
			"Wrong file placeholder value using input type",
			`{
				"query": "mutation ($input: FilesInput!) { uploadMultiInput(input: $input) }",
				"variables": { "input": { "files": [null, 50] } }
			}`,
			`{ "0": ["variables.input.files.0"], "1": ["variables.input.files.1"] }`,
			[][]byte{
				[]byte("File content 1"),
				[]byte("File content 2"),
			},
		},
	} {
		queryTest := queryTest // enable parallel sub-tests
		t.Run(queryTest.mess, func(t *testing.T) {
			t.Parallel()
			request, err := createMultipartRequest(
				[]byte(queryTest.operations),
				[]byte(queryTest.fileMap),
				queryTest.files...,
			)

			if err != nil {
				t.Error(err)
				return
			}

			// a recorder so we can check what the handler responded with
			responseRecorder := httptest.NewRecorder()

			// call the http hander
			gateway.GraphQLHandler(responseRecorder, request)

			// make sure we got an error code
			result := responseRecorder.Result()
			assert.NoError(t, result.Body.Close())
			assert.Equal(t, http.StatusUnprocessableEntity, result.StatusCode)
		})
	}

	t.Run("Not multipart request", func(t *testing.T) {
		t.Parallel()
		// the incoming request
		request := httptest.NewRequest("POST", "/graphql", strings.NewReader(`{ 
				"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
				"variables": { "someFile": null } 
			}`))
		request.Header.Set("Content-Type", "multipart/form-data")

		// a recorder so we can check what the handler responded with
		responseRecorder := httptest.NewRecorder()

		// call the http hander
		gateway.GraphQLHandler(responseRecorder, request)

		// make sure we got an error code
		result := responseRecorder.Result()
		assert.NoError(t, result.Body.Close())
		assert.Equal(t, http.StatusUnprocessableEntity, result.StatusCode)
	})

	t.Run("Unknown content-type", func(t *testing.T) {
		t.Parallel()
		// the incoming request
		request := httptest.NewRequest("POST", "/graphql", strings.NewReader(`{ 
				"query": "mutation ($someFile: Upload!) { upload(file: $someFile) }", 
				"variables": { "someFile": null } 
			}`))
		request.Header.Set("Content-Type", "foobar/form-data")

		// a recorder so we can check what the handler responded with
		responseRecorder := httptest.NewRecorder()

		// call the http hander
		gateway.GraphQLHandler(responseRecorder, request)

		// make sure we got an error code
		result := responseRecorder.Result()
		assert.NoError(t, result.Body.Close())
		assert.Equal(t, http.StatusUnprocessableEntity, result.StatusCode)
	})
}

func createMultipartRequest(operations, fileMap []byte, filesContent ...[]byte) (*http.Request, error) {
	var b = bytes.Buffer{}
	var fw io.Writer
	var err error

	w := multipart.NewWriter(&b)

	fw, err = w.CreateFormField("operations")
	if err != nil {
		return nil, err
	}

	_, err = fw.Write(operations)
	if err != nil {
		return nil, err
	}

	fw, err = w.CreateFormField("map")
	if err != nil {
		return nil, err
	}

	_, err = fw.Write(fileMap)
	if err != nil {
		return nil, err
	}

	for i, fileContent := range filesContent {
		fw, err = w.CreateFormFile(strconv.Itoa(i), fmt.Sprintf("file%d.txt", i))
		if err != nil {
			return nil, err
		}

		_, err = fw.Write(fileContent)
		if err != nil {
			return nil, err
		}
	}

	err = w.Close()
	if err != nil {
		return nil, err
	}

	// the incoming request
	request := httptest.NewRequest("POST", "/graphql", &b)
	request.Header.Set("Content-Type", w.FormDataContentType())

	return request, nil
}

func TestStaticPlaygroundHandler(t *testing.T) {
	t.Parallel()
	schema, err := graphql.LoadSchema(`
		type Query {
			allUsers: [String!]!
		}
	`)
	assert.NoError(t, err)

	// create gateway schema we can test against
	gateway, err := New([]*graphql.RemoteSchema{
		{Schema: schema, URL: "url1"},
	}, WithExecutor(ExecutorFunc(
		func(*ExecutionContext) (map[string]interface{}, error) {
			return map[string]interface{}{
				"Hello": "world",
			}, nil
		},
	)))

	if err != nil {
		t.Error(err.Error())
		return
	}

	t.Run("static UI", func(t *testing.T) {
		t.Parallel()
		request := httptest.NewRequest(http.MethodGet, "/graphql", strings.NewReader(""))
		responseRecorder := httptest.NewRecorder()
		gateway.StaticPlaygroundHandler(PlaygroundConfig{
			Endpoint: "some-url",
		}).ServeHTTP(responseRecorder, request)

		result := responseRecorder.Result()
		defer result.Body.Close()
		assert.Equal(t, http.StatusOK, result.StatusCode)

		assert.Contains(t, responseRecorder.Body.String(), "some-url")
	})

	t.Run("queries fail", func(t *testing.T) {
		t.Parallel()
		request := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`query { allUsers { firstName } }`))
		responseRecorder := httptest.NewRecorder()
		gateway.StaticPlaygroundHandler(PlaygroundConfig{}).ServeHTTP(responseRecorder, request)

		result := responseRecorder.Result()
		assert.NoError(t, result.Body.Close())
		assert.Equal(t, http.StatusMethodNotAllowed, result.StatusCode)
	})
}
