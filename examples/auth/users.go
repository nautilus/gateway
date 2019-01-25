package main

import (
	"errors"
	"log"
	"net/http"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
)

var Schema = `
	schema {
		query: Query
		mutation: Mutation
	}

	interface Node {
		id: ID!
	}

	type User implements Node {
		id: ID!
	}

	type Query {
		node(id: ID!): Node
	}

	type loginUserOutput {
		token: String!
	}

	type Mutation {
		loginUser(username: String!, password: String!): loginUserOutput!
	}
`

var users = []*User{
	{
		ID:       "1",
		Username: "username",
		Password: "password",
	},
}

// resolve the node field
func (r *Resolver) Node(args struct{ Id string }) *NodeResolver {
	return &NodeResolver{&UserResolver{users[0]}}
}

// handle the login mutation
func (r *Resolver) LoginUser(args struct {
	Username string
	Password string
}) (*LoginUserOutput, error) {
	// look for the user with the corresponding username and password
	for _, user := range users {
		if user.Username == args.Username && user.Password == args.Password {
			return &LoginUserOutput{string(user.ID)}, nil
		}
	}

	// we didn't find a matching username and password
	return nil, errors.New("Provided information was invalid")
}

//
//
// boilerplate for rest of API
//
//

type User struct {
	ID       graphql.ID
	Username string
	Password string
}

type LoginUserOutput struct {
	Tkn string
}

func (o *LoginUserOutput) Token() string {
	return o.Tkn
}

type Resolver struct{}

type node interface {
	ID() graphql.ID
}

type NodeResolver struct {
	node
}

func (node *NodeResolver) ToUser() (*UserResolver, bool) {
	user, ok := node.node.(*UserResolver)
	return user, ok
}

type UserResolver struct {
	user *User
}

func (u *UserResolver) ID() graphql.ID {
	return u.user.ID
}

// start the service
func main() {
	schema := graphql.MustParseSchema(Schema, &Resolver{})

	http.Handle("/query", &relay.Handler{Schema: schema})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
