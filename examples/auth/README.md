# Auth Example

This example showcases a typical approach to handling authorization and
authentication behind a gateway. In this example, there are 2 services behind the
gateway. One that's in charge of user information (including their password) and
another that handles a todo list.

The general flow goes something like:

- The user service defines a mutation called `loginUser` that checks if
  the combo is valid and responds with a token.

- Somehow (not shown here), the client holds onto this tokens and sends it
  with future requests to the gateway

- When the gateway receives a query, it looks for a token under the `Authorization`
  header, and if its present, sends the value as the `USER_ID` header when sending
  queries to the services.

\_ The other services know to look for that The other service looks
for the header value and

Keep in mind that while this example just sends the id of the user, you can send any
information you want to authorize the user.
