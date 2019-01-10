package gateway

import (
	"fmt"

	"github.com/vektah/gqlparser/ast"

	"github.com/alecaivazis/graphql-gateway/graphql"
)

// Schema is the top level entry for interacting with a gateway. It is responsible for merging a list of
// remote schemas into one, generating a query plan to execute based on an incoming request, and following
// that plan
type Schema struct {
	Sources  []graphql.RemoteSchema
	Schema   *ast.Schema
	Planner  QueryPlanner
	Executor Executor

	// the urls we have to visit to access certain fields
	fieldURLs FieldURLMap
}

// Execute takes a query string, executes it, and returns the response
func (s *Schema) Execute(query string) (map[string]interface{}, error) {
	// generate a query plan for the query
	plan, err := s.Planner.Plan(query, s.Schema, s.fieldURLs)
	if err != nil {
		return nil, err
	}

	// execute the plan and return the results
	return s.Executor.Execute(plan[0])
}

// NewSchema instantiates a new schema with the required stuffs.
func NewSchema(sources []graphql.RemoteSchema) (*Schema, error) {
	// grab the schemas to compute the sources
	sourceSchemas := []*ast.Schema{}
	for _, source := range sources {
		sourceSchemas = append(sourceSchemas, source.Schema)
	}

	// merge them into one
	schema, err := mergeSchemas(sourceSchemas)
	if err != nil {
		// if something went wrong during the merge, return the result
		return nil, err
	}

	// compute the locations for each field
	locations, err := fieldURLs(sources)
	if err != nil {
		// if something went wrong during the merge, return the result
		return nil, err
	}

	// return the resulting schema
	return &Schema{
		Sources:  sources,
		Schema:   schema,
		Planner:  &MinQueriesPlanner{},
		Executor: &ParallelExecutor{},

		// internal fields
		fieldURLs: locations,
	}, nil
}

func fieldURLs(schemas []graphql.RemoteSchema) (FieldURLMap, error) {
	// build the mapping of fields to urls
	locations := FieldURLMap{}

	// every schema we were given could define types
	for _, remoteSchema := range schemas {
		// each type defined by the schema can be found at remoteSchema.URL
		for name, typeDef := range remoteSchema.Schema.Types {
			// each field of each type can be found here
			for _, fieldDef := range typeDef.Fields {
				// register the location for the field
				locations.RegisterURL(name, fieldDef.Name, remoteSchema.URL)
			}
		}
	}

	// return the location map
	return locations, nil
}

// FieldURLMap holds the intformation for retrieving the valid locations one can find the value for the field
type FieldURLMap map[string][]string

// URLFor returns the list of locations one can find parent.field.
func (m FieldURLMap) URLFor(parent string, field string) ([]string, error) {
	// compute the key for the field
	key := m.keyFor(parent, field)

	// look up the value in the map
	value, exists := m[key]

	// if it doesn't exist
	if !exists {
		return []string{}, fmt.Errorf("Could not find location for %s", key)
	}

	// return the value to the caller
	return value, nil
}

// RegisterURL adds a new location to the list of possible places to find the value for parent.field
func (m FieldURLMap) RegisterURL(parent string, field string, location string) {
	// compute the key for the field
	key := m.keyFor(parent, field)

	// look up the value in the map
	_, exists := m[key]

	// if we haven't seen this key before
	if !exists {
		// create a new list
		m[key] = []string{location}
	} else {
		// we've seen this key before
		m[key] = append(m[key], location)
	}
}

func (m FieldURLMap) keyFor(parent string, field string) string {
	return fmt.Sprintf("%s.%s", parent, field)
}
