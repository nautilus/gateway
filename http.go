package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/nautilus/graphql"
)

type PersistedQuerySpecification struct {
	Version int    `json:"version"`
	Hash    string `json:"sha256Hash"`
}

// HTTPOperation is the incoming payload when sending POST requests to the gateway
type HTTPOperation struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
	Extensions    struct {
		QueryPlanCache *PersistedQuerySpecification `json:"persistedQuery"`
	} `json:"extensions"`
}

func formatErrors(err error) map[string]interface{} {
	return formatErrorsWithCode(nil, err, "UNKNOWN_ERROR")
}

func formatErrorsWithCode(data map[string]interface{}, err error, code string) map[string]interface{} {
	// the final list of formatted errors
	var errList graphql.ErrorList

	// if the err is itself an error list
	if !errors.As(err, &errList) {
		errList = graphql.ErrorList{
			graphql.NewError(code, err.Error()),
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
	operations, batchMode, parseStatusCode, payloadErr := parseRequest(r)

	// if there was an error retrieving the payload
	if payloadErr != nil {
		response := formatErrors(payloadErr)
		w.WriteHeader(parseStatusCode)
		err := json.NewEncoder(w).Encode(response)
		if err != nil {
			g.logger.Warn("Failed to encode error response:", err.Error())
		}
		return
	}

	/// Handle the operations regardless of the request method

	// we have to respond to each operation in the right order
	results := []map[string]interface{}{}

	// the status code to report
	statusCode := http.StatusOK

	for _, operation := range operations {
		// there might be a query plan cache key embedded in the operation
		cacheKey := ""
		if operation.Extensions.QueryPlanCache != nil {
			cacheKey = operation.Extensions.QueryPlanCache.Hash
		}

		// if there is no query or cache key
		if operation.Query == "" && cacheKey == "" {
			statusCode = http.StatusUnprocessableEntity
			results = append(
				results,
				formatErrorsWithCode(nil, errors.New("could not find query body"), "BAD_USER_INPUT"),
			)
			continue
		}

		// this might get mutated by the query plan cache so we have to pull it out
		requestContext := &RequestContext{
			Context:       r.Context(),
			Query:         operation.Query,
			OperationName: operation.OperationName,
			Variables:     operation.Variables,
			CacheKey:      cacheKey,
		}

		// Get the plan, and return a 400 if we can't get the plan
		plan, err := g.GetPlans(requestContext)
		if err != nil {
			response, err := json.Marshal(formatErrorsWithCode(nil, err, "GRAPHQL_VALIDATION_FAILED"))
			if err != nil {
				// if we couldn't serialize the response then we're in internal error territory
				response, err = json.Marshal(formatErrors(err))
				if err != nil {
					response, _ = json.Marshal(formatErrors(err))
				}
			}
			emitResponse(w, http.StatusBadRequest, string(response))
			return
		}

		// fire the query with the request context passed through to execution
		result, err := g.Execute(requestContext, plan)
		if err != nil {
			results = append(results, formatErrorsWithCode(result, err, "INTERNAL_SERVER_ERROR"))

			continue
		}

		// the result for this operation
		payload := map[string]interface{}{"data": result}

		// if there was a cache key associated with this query
		if requestContext.CacheKey != "" {
			// embed the cache key in the response
			payload["extensions"] = map[string]interface{}{
				"persistedQuery": map[string]interface{}{
					"sha265Hash": requestContext.CacheKey,
					"version":    "1",
				},
			}
		}

		// add this result to the list
		results = append(results, payload)
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
		response, err = json.Marshal(formatErrors(err))
		if err != nil {
			response, _ = json.Marshal(formatErrors(err))
		}
	}

	// send the result to the user
	emitResponse(w, statusCode, string(response))
}

// Parses request to operations (single or batch mode).
// Returns an error and an error status code if the request is invalid.
func parseRequest(r *http.Request) (operations []*HTTPOperation, batchMode bool, errStatusCode int, payloadErr error) {
	// this handler can handle multiple operations sent in the same query. Internally,
	// it models a single operation as a list of one.
	operations = []*HTTPOperation{}
	switch r.Method {
	case http.MethodGet:
		operations, payloadErr = parseGetRequest(r)
	case http.MethodPost:
		operations, batchMode, payloadErr = parsePostRequest(r)
	default:
		errStatusCode = http.StatusMethodNotAllowed
		payloadErr = errors.New(http.StatusText(http.StatusMethodNotAllowed))
	}
	if errStatusCode == 0 && payloadErr != nil { // ensure an error always results in a failed status code
		errStatusCode = http.StatusUnprocessableEntity
	}
	return
}

// Parses get request to list of operations
func parseGetRequest(r *http.Request) (operations []*HTTPOperation, payloadErr error) {
	parameters := r.URL.Query()

	// the operation we have to perform
	operation := &HTTPOperation{}

	// get the query parameter
	query, hasQuery := parameters["query"]
	if hasQuery {
		// save the query
		operation.Query = query[0]
	}

	// include operationName
	if variableInput, ok := parameters["variables"]; ok {
		variables := map[string]interface{}{}

		err := json.Unmarshal([]byte(variableInput[0]), &variables)
		if err != nil {
			payloadErr = errors.New("variables must be a json object")
		}

		// assign the variables to the payload
		operation.Variables = variables
	}

	// include operationName
	if operationName, ok := parameters["operationName"]; ok {
		operation.OperationName = operationName[0]
	}

	// if the request defined any extensions
	if extensionString, hasExtensions := parameters["extensions"]; hasExtensions {
		// copy the extension information into the operation
		if err := json.NewDecoder(strings.NewReader(extensionString[0])).Decode(&operation.Extensions); err != nil {
			payloadErr = err
		}
	}

	// include the query in the list of operations
	return []*HTTPOperation{operation}, payloadErr
}

// Parses post request (plain or multipart) to list of operations
func parsePostRequest(r *http.Request) (operations []*HTTPOperation, batchMode bool, payloadErr error) {
	contentTypes := strings.Split(r.Header.Get("Content-Type"), ";")
	if len(contentTypes) == 0 {
		return nil, false, errors.New("no content-type specified")
	}
	contentType := contentTypes[0]
	switch contentType {
	case "text/plain", "application/json", "":
		// read the full request body
		operationsJSON, err := io.ReadAll(r.Body)
		if err != nil {
			payloadErr = fmt.Errorf("encountered error reading body: %w", err)
			return
		}
		return parseOperations(operationsJSON)
	case "multipart/form-data":

		const maxPartSize = 32 << 20 // 32 Mebibytes
		parseErr := r.ParseMultipartForm(maxPartSize)
		if parseErr != nil {
			payloadErr = errors.New("error parse multipart request: " + parseErr.Error())
			return
		}

		operationsJSON := []byte(r.Form.Get("operations"))
		operations, batchMode, payloadErr = parseOperations(operationsJSON)

		var filePosMap map[string][]string
		if err := json.Unmarshal([]byte(r.Form.Get("map")), &filePosMap); err != nil {
			payloadErr = errors.New("error parsing file map " + err.Error())
			return
		}

		for filePos, paths := range filePosMap {
			file, header, err := r.FormFile(filePos)
			if err != nil {
				payloadErr = errors.New("file with index not found: " + filePos)
				return
			}

			fileMeta := graphql.Upload{
				File:     file,
				FileName: header.Filename,
			}

			if err := injectFile(operations, fileMeta, paths, batchMode); err != nil {
				payloadErr = err
				return
			}
		}
		return
	default:
		payloadErr = errors.New("unknown content-type: " + contentType)
		return
	}
}

// Parses json operations string
func parseOperations(operationsJSON []byte) (operations []*HTTPOperation, batchMode bool, payloadErr error) {
	// there are two possible options for receiving information from a post request
	// the first is that the user provides an object in the form of { query, variables, operationName }
	// the second option is a list of that object

	singleQuery := &HTTPOperation{}
	// if we were given a single object
	if err := json.Unmarshal(operationsJSON, &singleQuery); err == nil {
		// add it to the list of operations
		operations = append(operations, singleQuery)
		// we weren't given an object
	} else {
		// but we could have been given a list
		batch := []*HTTPOperation{}

		if err = json.Unmarshal(operationsJSON, &batch); err != nil {
			payloadErr = fmt.Errorf("encountered error parsing operationsJSON: %w", err)
		} else {
			operations = batch
		}

		// we're in batch mode
		batchMode = true
	}

	return operations, batchMode, payloadErr
}

// Adds file object to variables of respective operations in case of multipart request
func injectFile(operations []*HTTPOperation, file graphql.Upload, paths []string, batchMode bool) error {
	for _, path := range paths {
		var idx = 0
		parts := strings.Split(path, ".")
		if batchMode {
			idxVal, err := strconv.Atoi(parts[0])
			if err != nil {
				return err
			}
			idx = idxVal
			parts = parts[1:]
		}

		if parts[0] != "variables" {
			return errors.New("file locator doesn't have variables in it: " + path)
		}

		const minPathParts = 2
		if len(parts) < minPathParts {
			return errors.New("invalid number of parts in path: " + path)
		}

		variables := operations[idx].Variables

		// step through the path to find the file variable
		for i := 1; i < len(parts); i++ {
			val, ok := variables[parts[i]]
			if !ok {
				return fmt.Errorf("key not found in variables: %s", parts[i])
			}
			switch v := val.(type) {
			// if the path part is a map, then keep stepping through it
			case map[string]interface{}:
				variables = v
			// if we hit nil, then we have found the variable to replace with the file and have hit the end of parts
			case nil:
				variables[parts[i]] = file
			// if we find a list then find the the variable to replace at the parts index (supports: [Upload!]!)
			case []interface{}:
				// make sure the path contains another part before looking for an index
				if i+1 >= len(parts) {
					return fmt.Errorf("invalid number of parts in path: " + path)
				}

				// the next part in the path must be an index (ex: the "2" in: variables.input.files.2)
				index, err := strconv.Atoi(parts[i+1])
				if err != nil {
					return fmt.Errorf("expected numeric index: " + err.Error())
				}

				// index might not be within the bounds
				if index >= len(v) {
					return fmt.Errorf("file index %d out of bound %d", index, len(v))
				}
				fileVal := v[index]
				if fileVal != nil {
					return fmt.Errorf("expected nil value, got %v", fileVal)
				}
				v[index] = file

				// skip the final iteration through parts (skips the index definition, ex: the "2" in: variables.input.files.2)
				i++
			default:
				return fmt.Errorf("expected nil value, got %v", v) // possibly duplicate path or path to non-null variable
			}
		}
	}
	return nil
}

func emitResponse(w http.ResponseWriter, code int, response string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	fmt.Fprint(w, response)
}

// PlaygroundHandler returns a combined UI and API http.HandlerFunc.
// On POST requests, executes the designated query.
// On all other requests, shows the user an interface that they can use to interact with the API.
func (g *Gateway) PlaygroundHandler(w http.ResponseWriter, r *http.Request) {
	// on POSTs, we have to send the request to the graphqlHandler
	if r.Method == http.MethodPost {
		g.GraphQLHandler(w, r)
		return
	}

	// we are not handling a POST request so we have to show the user the playground
	err := writePlayground(w, PlaygroundConfig{
		Endpoint: r.URL.String(),
	})
	if err != nil {
		g.logger.Warn("failed writing playground UI:", err.Error())
	}
}

// StaticPlaygroundHandler returns a static UI http.HandlerFunc with custom configuration
func (g *Gateway) StaticPlaygroundHandler(config PlaygroundConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		err := writePlayground(w, config)
		if err != nil {
			g.logger.Warn("failed writing playground UI:", err.Error())
		}
	})
}
