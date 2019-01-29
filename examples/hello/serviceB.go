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
		lastName: String!
	}

	type Query {
		node(id: ID!): Node
	}
`

// the users by id
var users = map[string]*User{
	"1": {
		id:       "1",
		lastName: "Aivazis",
	},
}

// type resolvers

type User struct {
	id       graphql.ID
	lastName string
}

func (u *User) ID() graphql.ID {
	return u.id
}

func (u *User) LastName() string {
	return u.lastName
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

type queryB struct{}

func (q *queryB) Node(args struct{ ID string }) *NodeResolver {
	user := users[args.ID]

	if user != nil {
		return &NodeResolver{user}
	} else {
		return nil
	}
}

func main() {
	// attach the schema to the resolver object
	schema := graphql.MustParseSchema(Schema, &queryB{})

	// make sure we add the user info to the execution context
	http.Handle("/", &relay.Handler{Schema: schema})

	log.Fatal(http.ListenAndServe(":8081", nil))
}
