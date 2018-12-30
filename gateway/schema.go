package gateway

import (
	"fmt"

	"github.com/vektah/gqlparser/ast"
)

// Schema is the top level entry for interacting with a gateway. It is responsible for merging a list of
// remote schemas into one, generating a query plan to execute based on an incoming request, and following
// that plan
type Schema struct {
	Sources []RemoteSchema
	Schema  *ast.Schema
	Planner QueryPlanner

	// the urls we have to visit to access certain fields
	fieldLocations FieldLocationMap
}

// RemoteSchema encapsulates a particular schema that can be executed by sending network requests to the
// specified URL.
type RemoteSchema struct {
	Schema   *ast.Schema
	Location string
}

// Plan returns the query plan for the incoming query
func (s *Schema) Plan(query string) (*QueryPlan, error) {
	return s.Planner.Plan(query, s.fieldLocations)
}

// NewSchema instantiates a new schema with the required stuffs.
func NewSchema(sources []RemoteSchema) (*Schema, error) {
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
	locations, err := fieldLocations(sources)
	if err != nil {
		// if something went wrong during the merge, return the result
		return nil, err
	}

	// return the resulting schema
	return &Schema{
		Sources: sources,
		Schema:  schema,
		Planner: &NaiveQueryPlanner{},

		// internal fields
		fieldLocations: locations,
	}, nil
}

func fieldLocations(schemas []RemoteSchema) (FieldLocationMap, error) {
	// build the mapping of fields to urls
	locations := FieldLocationMap{}

	// every schema we were given could define types
	for _, remoteSchema := range schemas {
		// each type defined by the schema can be found at remoteSchema.Location
		for name, typeDef := range remoteSchema.Schema.Types {
			// each field of each type can be found here
			for _, fieldDef := range typeDef.Fields {
				// register the location for the field
				locations.RegisterLocation(name, fieldDef.Name, remoteSchema.Location)
			}
		}
	}

	// return the location map
	return locations, nil
}

// FieldLocationMap holds the intformation for retrieving the valid locations one can find the value for the field
type FieldLocationMap map[string][]string

// LocationFor returns the list of locations one can find parent.field.
func (m FieldLocationMap) LocationFor(parent string, field string) ([]string, error) {
	// compute the key for the field
	key := m.keyFor(parent, field)

	// look up the value in the map
	value, exists := m[key]

	// if it doesn't exist
	if !exists {
		return []string{}, fmt.Errorf("Could not find location for key %s", key)
	}

	// return the value to the caller
	return value, nil
}

// RegisterLocation adds a new location to the list of possible places to find the value for parent.field
func (m FieldLocationMap) RegisterLocation(parent string, field string, location string) {
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

func (m FieldLocationMap) keyFor(parent string, field string) string {
	return fmt.Sprintf("%s.%s", parent, field)
}
