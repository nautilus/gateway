package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nautilus/gateway"
	"github.com/nautilus/graphql"
	"github.com/vektah/gqlparser/ast"
)

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
			context.WithValue(r.Context(), "user-id", tokenValue),
		))
	})
}

// the next thing we need to do is to modify the network requests to our services.
// To do this, we have to define a middleware that pulls the id of the user out
// of the context of the incoming request and sets it as the USER_ID header.
var forwardUserID = gateway.RequestMiddleware(func(r *http.Request) error {
	// the initial context of the request is set as the same context
	// provided by net/http

	// we are safe to extract the value we saved in context and set it as the outbound header
	if userID := r.Context().Value("user-id"); userID != nil {
		r.Header.Set("USER_ID", userID.(string))
	}

	// return the modified request
	return nil
})

// we can also define a field at the root of the API in order to resolve fields
// for the current user
var viewerField = &gateway.QueryField{
	Name: "viewer",
	Type: ast.NamedType("User", &ast.Position{}),
	Resolver: func(ctx context.Context, args map[string]interface{}) (string, error) {
		// for now just return the value in context
		return ctx.Value("user-id").(string), nil
	},
}

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
	gw, err := gateway.New(schemas, gateway.WithMiddlewares(forwardUserID), gateway.WithQueryFields(viewerField))
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
