package gateway

import (
	"net/http"

	"github.com/alecaivazis/graphql-gateway/graphql"
)

// Middleware are things that can modify a gateway normal execution
type Middleware interface {
	Middleware()
}

// MiddlewareList is a list of Middlewares
type MiddlewareList []Middleware

// ApplyRequestMiddlewares iterates over the list of middlewares and applies any that
// affect a query request
func (l MiddlewareList) ApplyRequestMiddlewares(r *http.Request) (*http.Request, error) {
	// look for each query request Middleware and add it to the list
	for _, Middleware := range l {
		if rMiddleware, ok := Middleware.(RequestMiddleware); ok {
			// invoke the Middleware
			newValue, err := rMiddleware(r)
			if err != nil {
				return nil, err
			}

			// hold onto the new value to thread it through again
			r = newValue
		}
	}

	// return the request
	return r, nil
}

// RequestMiddleware is a middleware that can modify outbound requests to services
type RequestMiddleware graphql.NetworkMiddleware

// Middleware marks RequestMiddleware as a valid middleware
func (p RequestMiddleware) Middleware() {}
