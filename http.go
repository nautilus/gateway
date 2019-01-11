package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *Schema) GraphQLHandler(w http.ResponseWriter, r *http.Request) {
	query, ok := r.URL.Query()["query"]
	if !ok {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprintf(w, "Please send a query to this endpoint.")
		return
	}

	// fire the query
	result, err := s.Execute(query[0])
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Encountered error during execution: %s", err.Error())
		return
	}

	payload, err := json.Marshal(result)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Encountered error marshaling response: %s", err.Error())
		return
	}

	fmt.Fprintf(w, string(payload))
}
