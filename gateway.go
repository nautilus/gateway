package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vektah/gqlparser/ast"

	"github.com/nautilus/graphql"
)

type contextKey int

// Gateway is the top level entry for interacting with a gateway. It is responsible for merging a list of
// remote schemas into one, generating a query plan to execute based on an incoming request, and following
// that plan
type Gateway struct {
	sources     []*graphql.RemoteSchema
	schema      *ast.Schema
	planner     QueryPlanner
	executor    Executor
	merger      Merger
	middlewares MiddlewareList

	// group up the list of middlewares at startup to avoid it during execution
	requestMiddlewares  []graphql.NetworkMiddleware
	responseMiddlewares []ResponseMiddleware

	// the urls we have to visit to access certain fields
	fieldURLs FieldURLMap
}

// Execute takes a query string, executes it, and returns the response
func (g *Gateway) Execute(requestContext context.Context, query string, variables map[string]interface{}) (map[string]interface{}, error) {
	// generate a query plan for the query
	plan, err := g.planner.Plan(query, g.schema, g.fieldURLs)
	if err != nil {
		return nil, err
	}

	// build up the execution context
	ctx := &ExecutionContext{
		RequestContext:     requestContext,
		RequestMiddlewares: g.requestMiddlewares,
		Plan:               plan[0],
		Variables:          variables,
	}

	// TODO: handle plans of more than one query
	// execute the plan and return the results
	result, err := g.executor.Execute(ctx)
	if err != nil {
		return nil, err
	}

	// now that we have our response, throw it through the list of middlewarse
	for _, ware := range g.responseMiddlewares {
		if err := ware(ctx, result); err != nil {
			return nil, err
		}
	}

	// we're done here
	return result, nil
}

// New instantiates a new schema with the required stuffs.
func New(sources []*graphql.RemoteSchema, configs ...Configurator) (*Gateway, error) {
	// if there are no source schemas
	if len(sources) == 0 {
		return nil, errors.New("a gateway must have at least one schema")
	}

	// set any default values before we start doing stuff with it
	gateway := &Gateway{
		sources:  sources,
		planner:  &MinQueriesPlanner{},
		executor: &ParallelExecutor{},
		merger:   MergerFunc(mergeSchemas),
	}

	// pass the gateway through any configurators
	for _, config := range configs {
		config(gateway)
	}

	// find the field URLs before we merge schemas. We need to make sure to include
	// the fields defined by the gateway's internal schema
	urls := fieldURLs(sources, true).Concat(
		fieldURLs([]*graphql.RemoteSchema{internalSchema}, false),
	)

	// grab the schemas within each source
	sourceSchemas := []*ast.Schema{}
	for _, source := range sources {
		sourceSchemas = append(sourceSchemas, source.Schema)
	}
	sourceSchemas = append(sourceSchemas, internalSchema.Schema)

	// merge them into one
	schema, err := gateway.merger.Merge(sourceSchemas)
	if err != nil {
		// if something went wrong during the merge, return the result
		return nil, err
	}

	// pull out the executor middlewares once here so that we don't have
	// to do it on every execute
	requestMiddlewares := []graphql.NetworkMiddleware{}
	// before we do anything that the user tells us to, we have to scrub the fields
	responseMiddlewares := []ResponseMiddleware{scrubFields}
	for _, mware := range gateway.middlewares {
		switch mware := mware.(type) {
		case ResponseMiddleware:
			responseMiddlewares = append(responseMiddlewares, mware)
		case RequestMiddleware:
			requestMiddlewares = append(requestMiddlewares, graphql.NetworkMiddleware(mware))
		default:
		}
	}

	// assign the computed values
	gateway.schema = schema
	gateway.fieldURLs = urls
	gateway.requestMiddlewares = requestMiddlewares
	gateway.responseMiddlewares = responseMiddlewares

	// we're done here
	return gateway, nil
}

// Configurator is a function to be passed to New that configures the
// resulting schema
type Configurator func(*Gateway)

// WithPlanner returns a Configurator that sets the planner of the gateway
func WithPlanner(p QueryPlanner) Configurator {
	return func(g *Gateway) {
		g.planner = p
	}
}

// WithExecutor returns a Configurator that sets the executor of the gateway
func WithExecutor(e Executor) Configurator {
	return func(g *Gateway) {
		g.executor = e
	}
}

// WithMerger returns a Configurator that sets the merger of the gateway
func WithMerger(m Merger) Configurator {
	return func(g *Gateway) {
		g.merger = m
	}
}

// WithMiddleware returns a Configurator that adds middlewares to the gateway
func WithMiddleware(middlewares ...Middleware) Configurator {
	return func(g *Gateway) {
		g.middlewares = append(g.middlewares, middlewares...)
	}
}

func fieldURLs(schemas []*graphql.RemoteSchema, stripInternal bool) FieldURLMap {
	// build the mapping of fields to urls
	locations := FieldURLMap{}

	// every schema we were given could define types
	for _, remoteSchema := range schemas {
		// each type defined by the schema can be found at remoteSchema.URL
		for name, typeDef := range remoteSchema.Schema.Types {
			if !strings.HasPrefix(typeDef.Name, "__") || !stripInternal {
				// each field of each type can be found here
				for _, fieldDef := range typeDef.Fields {
					// if the field is not an introspection field
					if !(name == "Query" && strings.HasPrefix(fieldDef.Name, "__")) {
						locations.RegisterURL(name, fieldDef.Name, remoteSchema.URL)
					} else {
						// its an introspection name
						if !stripInternal {
							// register the location for the field
							locations.RegisterURL(name, fieldDef.Name, remoteSchema.URL)
						}
					}

				}
			}
		}
	}

	// return the location map
	return locations
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

// Concat returns a new field map url whose entries are the union of both maps
func (m FieldURLMap) Concat(other FieldURLMap) FieldURLMap {
	for key, value := range other {
		// if we have seen the location before
		if prevValue, ok := m[key]; ok {
			// add the values to the internal registry
			m[key] = append(prevValue, value...)

			// we havent' seen the key before
		} else {
			m[key] = value
		}
	}

	// return the
	return m
}

// RegisterURL adds a new location to the list of possible places to find the value for parent.field
func (m FieldURLMap) RegisterURL(parent string, field string, locations ...string) {
	for _, location := range locations {
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
}

func (m FieldURLMap) keyFor(parent string, field string) string {
	return fmt.Sprintf("%s.%s", parent, field)
}
