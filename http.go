package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/nautilus/graphql"
)

// QueryPOSTBody is the incoming payload when sending POST requests to the gateway
type QueryPOSTBody struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
}

func writeErrors(err error, w http.ResponseWriter) {
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

	response, err := json.Marshal(map[string]interface{}{
		"errors": errList,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		writeErrors(err, w)
		return
	}

	w.Write(response)
}

// GraphQLHandler returns a http.HandlerFunc that should be used as the
// primary endpoint for the gateway API. The endpoint will respond
// to queries on both GET and POST requests.
func (g *Gateway) GraphQLHandler(w http.ResponseWriter, r *http.Request) {
	// a place to store query params
	payload := QueryPOSTBody{}

	// the error we have encountered when extracting query input
	var payloadErr error

	// if we got a GET request
	if r.Method == http.MethodGet {
		parameters := r.URL.Query()
		// get the query parameter
		if query, ok := parameters["query"]; ok {
			payload.Query = query[0]

			// include operationName
			if variableInput, ok := parameters["variables"]; ok {
				variables := map[string]interface{}{}

				err := json.Unmarshal([]byte(variableInput[0]), &variables)
				if err != nil {
					payloadErr = errors.New("variables must be a json object")
				}

				// assign the variables to the payload
				payload.Variables = variables
			}

			// include operationName
			if operationName, ok := parameters["operationName"]; ok {
				payload.OperationName = operationName[0]
			}
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

		err = json.Unmarshal(body, &payload)
		if err != nil {
			payloadErr = fmt.Errorf("encountered error parsing body: %s", err.Error())
		}
	}

	// if there was an error retrieving the payload
	if payloadErr != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		writeErrors(payloadErr, w)
		return
	}

	// if we dont have a query
	if payload.Query == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		writeErrors(errors.New("could not find a query in request payload"), w)
		return
	}
	

	// fire the query with the request context passed through to execution
	result, err := g.Execute(r.Context(), payload.Query, payload.Variables)
	if err != nil {
		writeErrors(err, w)
		return
	}

	response, err := json.Marshal(map[string]interface{}{
		"data": result,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		writeErrors(err, w)
		return
	}

	// send the result to the user
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
