The `Gateway` is made up of 3 interface-driven algorithm:

- the `Merger` is responsible for taking a list of `graphql.RemoteSchema` and merging them into
  a single schema. Along the way, it keeps track of what fields are defined at what locations so
  that the `Planner` can do its job.

- the `Planner`s job is to take an incoming query and construct a query plan that will resolve
  the requested query. These query plans have a `Queryer` embedded in them for more flexibility.
  Any kind of query-time changes have to be made to the executor since planning happens once for
  a given query.

- the `Executor` then takes the query plan and executes the query with the provided variables
  and context representing the current user.

At the moment, `graphql-gateway` only provides a single implementation of `Merger`, `Planner`, and
`Executor`. If you have a custom implementation, you can configure the gateway to use them at
construction time:

```golang
gateway.New(schemas, gateway.WithPlanner(MyCustomPlanner{}), gateway.WithExecutor(MyCustomExecutor{}))
```
