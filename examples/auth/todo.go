package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
)

var Schema = `
	schema {
		query: Query
	}

	type Query {
		viewerTodos: [Todo!]!
	}

	type Todo {
		title: String!
		done: Boolean!
	}
`

type Todo struct {
	title string
	done  bool
}

func (t *Todo) Title() string {
	return t.title
}
func (t *Todo) Done() bool {
	return t.done
}

// the list of todos for each user
var todos = map[string][]*Todo{
	"1": []*Todo{
		{
			title: "Foo 1",
		},
		{
			title: "Bar 1",
		},
	},
	"2": []*Todo{
		{
			title: "Foo 2",
		},
		{
			title: "Bar 2",
		},
		{
			title: "Baz 2",
		},
	},
	"3": []*Todo{},
}

type query struct{}

// look up the list of todos for the current user
func (r *query) ViewerTodos(ctx context.Context) ([]*Todo, error) {
	// grab the current user from the context
	userID, ok := ctx.Value("user-id").(string)
	if !ok {
		return nil, errors.New("user ID was not a string")
	}

	fmt.Println("Hello", userID)

	// return the todos for the appropriate user
	return todos[userID], nil
}

// addUserInfo is a handler middleware that extracts the appropriate header and sets it
// to a known place in context
func addUserInfo(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// before we invoke the graphql endpoint we need to add the current
		// user to the context

		// look up the value of the USER_ID header
		userID := r.Header.Get("USER_ID")

		// set the value in context
		ctx := context.WithValue(r.Context(), "user-id", userID)

		// pass the handler args to the wrapped handler
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

func main() {
	schema := graphql.MustParseSchema(Schema, &query{})

	// make sure we add the user info to the execution context
	http.Handle("/", addUserInfo(&relay.Handler{Schema: schema}))

	log.Fatal(http.ListenAndServe(":8081", nil))

}
