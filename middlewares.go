package gateway

import (
	"github.com/nautilus/graphql"
)

// Middleware are things that can modify a gateway normal execution
type Middleware interface {
	Middleware()
}

// MiddlewareList is a list of Middlewares
type MiddlewareList []Middleware

// RequestMiddleware is a middleware that can modify outbound requests to services
type RequestMiddleware graphql.NetworkMiddleware

// Middleware marks RequestMiddleware as a valid middleware
func (p RequestMiddleware) Middleware() {}
