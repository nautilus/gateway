package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nautilus/graphql"
)

// HTTPOperation is the incoming payload when sending POST requests to the gateway
type HTTPOperation struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
	Extensions    struct {
		PersistedQuery struct {
			Version int    `json:"version"`
			Hash    string `json:"sha256Hash"`
		} `json:"persistedQuery"`
	} `json:"extensions"`
}

func formatErrors(data map[string]interface{}, err error) map[string]interface{} {
	// the final list of formatted errors
	var errList graphql.ErrorList

	// if the err is itself an error list
	if list, ok := err.(graphql.ErrorList); ok {
		errList = list
	} else {
		errList = graphql.ErrorList{
			&graphql.Error{
				Message: err.Error(),
			},
		}
	}

	return map[string]interface{}{
		"data":   data,
		"errors": errList,
	}
}

// GraphQLHandler returns a http.HandlerFunc that should be used as the
// primary endpoint for the gateway API. The endpoint will respond
// to queries on both GET and POST requests. POST requests can either be
// a single object with { query, variables, operationName } or a list
// of that object.
func (g *Gateway) GraphQLHandler(w http.ResponseWriter, r *http.Request) {
	// this handler can handle multiple operations sent in the same query. Internally,
	// it modules a single operation as a list of one.
	operations := []*HTTPOperation{}

	// the error we have encountered when extracting query input
	var payloadErr error
	// make our lives easier. track if we're in batch mode
	batchMode := false

	// if we got a GET request
	if r.Method == http.MethodGet {
		parameters := r.URL.Query()
		// get the query parameter
		if query, ok := parameters["query"]; ok {
			// build a query obj
			query := &HTTPOperation{
				Query: query[0],
			}

			// include operationName
			if variableInput, ok := parameters["variables"]; ok {
				variables := map[string]interface{}{}

				err := json.Unmarshal([]byte(variableInput[0]), &variables)
				if err != nil {
					payloadErr = errors.New("variables must be a json object")
				}

				// assign the variables to the payload
				query.Variables = variables
			}

			// include operationName
			if operationName, ok := parameters["operationName"]; ok {
				query.OperationName = operationName[0]
			}

			//

			// add the query to the list of operations
			operations = append(operations, query)

		} else {
			// there was no query parameter
			payloadErr = errors.New("must include query as parameter")
		}
		// or we got a POST request
	} else if r.Method == http.MethodPost {
		// read the full request body
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			payloadErr = fmt.Errorf("encountered error reading body: %s", err.Error())
		}

		// there are two possible options for receiving information from a post request
		// the first is that the user provides an object in the form of { query, variables, operationName }
		// the second option is a list of that object

		singleQuery := &HTTPOperation{}
		// if we were given a single object
		if err = json.Unmarshal(body, &singleQuery); err == nil {
			// add it to the list of operations
			operations = append(operations, singleQuery)
			// we weren't given an object
		} else {
			// but we could have been given a list
			batch := []*HTTPOperation{}

			if err = json.Unmarshal(body, &batch); err != nil {
				payloadErr = fmt.Errorf("encountered error parsing body: %s", err.Error())
			} else {
				operations = batch
			}

			// we're in batch mode
			batchMode = true
		}
	}

	// if there was an error retrieving the payload
	if payloadErr != nil {
		// stringify the response
		response, _ := json.Marshal(formatErrors(map[string]interface{}{}, payloadErr))

		// send the error to the user
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write(response)
		return
	}

	/// Handle the operations regardless of the request method

	// we have to respond to each operation in the right order
	results := []map[string]interface{}{}

	// the status code to report
	statusCode := http.StatusOK

	for _, operation := range operations {
		// the result of the operation
		result := map[string]interface{}{}

		// the result of the operation
		if operation.Query == "" {
			statusCode = http.StatusUnprocessableEntity
			results = append(results, formatErrors(map[string]interface{}{}, errors.New("could not find query body")))
			continue
		}

		// fire the query with the request context passed through to execution
		result, err := g.Execute(r.Context(), operation.Query, operation.Variables)
		if err != nil {
			results = append(results, formatErrors(map[string]interface{}{}, err))
			continue
		}

		// add this result to the list
		results = append(results, map[string]interface{}{"data": result})
	}

	// the final result depends on whether we are executing in batch mode or not
	var finalResponse interface{}
	if batchMode {
		finalResponse = results
	} else {
		finalResponse = results[0]
	}

	// serialized the response
	response, err := json.Marshal(finalResponse)
	if err != nil {
		// if we couldn't serialize the response then we're in internal error territory
		statusCode = http.StatusInternalServerError
		response, err = json.Marshal(formatErrors(map[string]interface{}{}, err))
		if err != nil {
			response, _ = json.Marshal(formatErrors(map[string]interface{}{}, err))
		}
	}

	// send the result to the user
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	fmt.Fprint(w, string(response))
}

// PlaygroundHandler returns a http.HandlerFunc which on GET requests shows
// the user an interface that they can use to interact with the API. On
// POSTs the endpoint executes the designated query
func (g *Gateway) PlaygroundHandler(w http.ResponseWriter, r *http.Request) {
	// on POSTs, we have to send the request to the graphqlHandler
	if r.Method == http.MethodPost {
		g.GraphQLHandler(w, r)
		return
	}

	// we are not handling a POST request so we have to show the user the playground
	w.Write(playgroundContent)
}
