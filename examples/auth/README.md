# Authentication Example

This example showcases a typical approach to handling authorization and
authentication behind a gateway. In this example, there are 2 services apart from
the gateway itself. One service is in charge of user information (including their
password) and the other handles a todo list. The intent is that a user logs in and
can see their specific todo list.

The general flow goes something like:

- The user service defines a mutation called `loginUser` that [checks if
  the credentials are valid](https://github.com/nautilus/gateway/blob/master/examples/auth/users.go#L66) and responds with a token.

- Somehow (not shown here), the client holds onto this tokens and sends it
  with future requests to the gateway under the `Authorization` header.

- When the gateway receives a query, it [looks for the token](https://github.com/nautilus/gateway/blob/master/examples/auth/gateway.go#L15-L29) and if its present,
  sends the value as the `USER_ID` header when sending queries to the services.

- The other services [uses the header value](https://github.com/nautilus/gateway/blob/master/examples/auth/todo.go#L89) to perform whatever user-specific logic is
  required.
  
- The current user can query for their User record with the [viewer gateway field]() (coming soon)

Keep in mind that this demo should not be taken as an example of a secure
authorization system. Its purpose is just to illustrate how one can pass
pass user-specific information onto the backing services.

## Running the example

To run the example, start the services defined in `users.go` and `todo.go` first by running
`go run <file name>` from this directory. You'll have to run them in separate terminals.
Then in a third terminal, start the `gateway.go` and visit http://localhost:4000 which
should show you a playground to interact with.

## User Credentials

In this example, there are 3 users (numbered 1,2,3) with credentials that take the form
`username1`/`password1`. Each of them has a unique set of todo items.
