package gateway

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

// QueryPOSTBody is the incoming payload when sending POST requests to the gateway
type QueryPOSTBody struct {
	Query         string `json:"query"`
	Variables     string `json:"variables"`
	OperationName string `json:"operationName"`
}

// GraphQLHandler returns a http.HandlerFunc that should be used as the
// primary endpoint for the gateway API. If withGraphiql is set to true,
// the endpoint will show a on GET requests, and respond to queries on
// POSTs only. If withGraphiql is set to false, the endpoint will respond
// to queries on both GET and POST requests.
func (s *Schema) GraphQLHandler(withGraphiql bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// a place to store query params
		payload := QueryPOSTBody{}

		// read the full request body
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprintf(w, "Encountered error reading body: %s", err.Error())
			return
		}

		err = json.Unmarshal(body, &payload)
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprintf(w, "Encountered error parsing body: %s", err.Error())
			return
		}

		// if we dont have a query
		if payload.Query == "" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, "Could not find a query in payload")
			return
		}

		// fire the query
		result, err := s.Execute(payload.Query)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Encountered error during execution: %s", err.Error())
			return
		}

		response, err := json.Marshal(result)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Encountered error marshaling response: %s", err.Error())
			return
		}

		fmt.Fprintf(w, string(response))
	}
}
