package gateway

import (
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
	return nil
}
