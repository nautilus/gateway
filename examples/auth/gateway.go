package main

import (
	"context"
	"fmt"
	"net/http"

	gateway "github.com/alecaivazis/graphql-gateway"
	"github.com/alecaivazis/graphql-gateway/graphql"
)

type contextKey int

const userIDCtx contextKey = iota

// the first thing we need to define is a middleware for our handler
// that grabs the Authorization header and sets the context value for
// our user id
func withUserInfo(handler http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// look up the value of the Authorization header
		tokenValue := r.Header.Get("Authorization")

		// here is where you would perform some kind of validation on the token
		// but we're going to skip that for this example and just save it as the
		// id directly. PLEASE, DO NOT DO THIS IN PRODUCTION.

		// invoke the handler with the new context
		handler.ServeHTTP(w, r.WithContext(
			context.WithValue(r.Context(), userIDCtx, tokenValue),
		))
	})
}

// the next thing we need to do is to modify the network requests to our services.
// To do this, we will define a Middleware that pulls the id of the user out
// of the context of the incoming request set it as the USER_ID header in the outbound request.
var forwardUserID = gateway.RequestMiddleware(func(r *http.Request) (*http.Request, error) {
	// the initial context of the request is set as the same context
	// provided by net/http

	// we are safe to extract the value we saved in context and set it as the outbound header
	r.Header.Set("USER_ID", r.Context().Value(userIDCtx).(string))

	// return the modified request
	return r, nil
})

func main() {
	// introspect the apis
	schemas, err := graphql.IntrospectRemoteSchemas(
		"http://localhost:8080/",
		"http://localhost:8081/",
	)
	if err != nil {
		panic(err)
	}

	// create the gateway instance
	gw, err := gateway.New(schemas, gateway.WithMiddlewares(forwardUserID))
	if err != nil {
		panic(err)
	}

	// add the playground endpoint to the router
	http.HandleFunc("/graphql", withUserInfo(gw.PlaygroundHandler))

	// start the server
	fmt.Println("Starting server")
	err = http.ListenAndServe(":3001", nil)
	if err != nil {
		fmt.Println(err.Error())
	}
}
