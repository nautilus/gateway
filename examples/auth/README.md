# Auth Example

This example showcases a typical approach to handling authorization and
authentication behind a gateway. In this example, there are 2 services apart from
the gateway itself. One service is in charge of user information (including their
password) and the other handles a todo list. The intent is that a user logs in and
can see their specific todo list.

The general flow goes something like:

- The user service defines a mutation called `loginUser` that [checks if
  the combo is valid](https://github.com/AlecAivazis/graphql-gateway/blob/examples/examples/auth/users.go#L53) and responds with a token.

- Somehow (not shown here), the client holds onto this tokens and sends it
  with future requests to the gateway under the `Authorization` header.

- When the gateway receives a query, it [looks for the token]() and if its present,
  sends the value as the `USER_ID` header when sending queries to the services.

- The other services [uses the header value]() to perform whatever user-specific logic is
  required.

Keep in mind that this demo should not be taken as an example of a secure
authorization system. Its purpose is just to illustrate how one can pass
pass user-specific information onto the backing services.
