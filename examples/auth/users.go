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
		Username: "username1",
		Password: "password1",
	},
	{
		ID:       "2",
		Username: "username2",
		Password: "password2",
	},
	{
		ID:       "3",
		Username: "username3",
		Password: "password3",
	},
}

// LoginUser is the primary userResolver for the mutation to log a user in.
// It's resposibility it to check that the credentials are correct
// and return a string that will be used to identity the user later.
func (r *Resolver) LoginUser(args struct {
	Username string
	Password string
}) (*LoginUserOutput, error) {
	// look for the user with the corresponding username and password
	for _, user := range users {
		if user.Username == args.Username && user.Password == args.Password {
			// return the token that the client will send back to us to claim the identity
			return &LoginUserOutput{
				Tkn: string(user.ID),
			}, nil
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

type userResolver struct{}

func (r *userResolver) Node(args struct{ ID string }) *NodeResolver {
	return &NodeResolver{&UserResolver{users[0]}}
}

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
	schema := graphql.MustParseSchema(Schema, &userResolver{})

	http.Handle("/query", &relay.Handler{Schema: schema})

	log.Fatal(http.ListenAndServe(":8080", nil))
}
