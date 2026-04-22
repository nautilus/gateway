package gateway

import (
	"maps"

	"github.com/nautilus/gateway/internal/execresult"
	"github.com/nautilus/graphql"
)

// Middleware are things that can modify a gateway normal execution
type Middleware interface {
	Middleware()
}

// ExecutionMiddleware are things that interject in the execution process
type ExecutionMiddleware interface {
	ExecutionMiddleware()
}

// MiddlewareList is a list of Middlewares
type MiddlewareList []Middleware

// RequestMiddleware is a middleware that can modify outbound requests to services
type RequestMiddleware graphql.NetworkMiddleware

// Middleware marks RequestMiddleware as a valid middleware
func (p RequestMiddleware) Middleware() {}

// ResponseMiddleware is a middleware that can modify the
// response before it is serialized and sent to the user
type ResponseMiddleware func(ctx *ExecutionContext, response map[string]interface{}) error

// Middleware marks ResponseMiddleware as a valid middleware
func (p ResponseMiddleware) Middleware() {}

// ExecutionMiddleware marks ResponseMiddleware as a valid execution middleware
func (p ResponseMiddleware) ExecutionMiddleware() {}

// scrubInsertionIDs removes the fields from the final response that the user did not
// explicitly ask for
func scrubInsertionIDs(ctx *ExecutionContext, response map[string]interface{}) error {
	// there are many fields to scrub
	responseObj, responseIsNonNull := execresult.NewObjectFromMap(response)
	if !responseIsNonNull {
		return nil
	}
	for field, locations := range ctx.Plan.FieldsToScrub {
		for _, location := range locations {
			// look for the insertion points in the response for the field
			insertionPoints, err := executorFindInsertionPoints(ctx, location, ctx.Plan.Operation.SelectionSet, responseObj, [][]string{{}}, ctx.Plan.FragmentDefinitions)
			if err != nil {
				return err
			}

			// each insertion point needs to be cleaned up
			for _, point := range insertionPoints {
				// extract the obj at that point
				value, err := executorExtractValue(ctx, responseObj, point)
				if err != nil {
					return err
				}
				value.Delete(field)
			}
		}
	}
	// We cannot replace response by pointer without changing function signature, which would break backward compatibility, so clear it and add keys back in
	for key := range response {
		delete(response, key)
	}
	maps.Copy(response, responseObj.ToMap())

	// the first thing we have to do is flatten all of the fragments into a single
	return nil
}
