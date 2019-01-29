package main

import (
	"log"
	"net/http"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
)

var Schema = `
	schema {
		query: Query
	}

	interface Node {
		id: ID!
	}

	type User implements Node {
		id: ID!
		firstName: String!
	}

	type Query {
		node(id: ID!): Node
		allUsers: [User!]!
	}
`

// the users by id
var users = map[string]*User{
	"1": {
		id:        "1",
		firstName: "Alec",
	},
}

// type resolvers

type User struct {
	id        graphql.ID
	firstName string
}

func (u *User) ID() graphql.ID {
	return u.id
}

func (u *User) FirstName() string {
	return u.firstName
}

type Node interface {
	ID() graphql.ID
}

type NodeResolver struct {
	node Node
}

func (n *NodeResolver) ID() graphql.ID {
	return n.node.ID()
}

func (n *NodeResolver) ToUser() (*User, bool) {
	user, ok := n.node.(*User)
	return user, ok
}

// query resolvers

type queryA struct{}

func (q *queryA) Node(args struct{ ID string }) *NodeResolver {
	user := users[args.ID]

	if user != nil {
		return &NodeResolver{user}
	} else {
		return nil
	}
}

func (q *queryA) AllUsers() []*User {
	// build up a list of all the users
	userSlice := []*User{}

	for _, user := range users {
		userSlice = append(userSlice, user)
	}

	return userSlice
}

func main() {
	// attach the schema to the resolver object
	schema := graphql.MustParseSchema(Schema, &queryA{})

	// make sure we add the user info to the execution context
	http.Handle("/", &relay.Handler{Schema: schema})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
