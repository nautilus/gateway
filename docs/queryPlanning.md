## Query Planning

Given schema1 located at `location1`:
```graphql
type User { 
    firstName:String!
}

type Query {
    allUsers: [User!]!
}
```

And `schema2` located at `location2`:
```graphql
type User { 
    lastName: String!
}
```

A query that looks like:
```gql
query AllUsersQuery { 
    allUsers { 
        firstName
        lastName
    }
}
```

results in a 2-step plan:
```json5
[
  {
      type: "Query",
      url: location1,
      insertionPoint: [],
      selection: `{
         allUsers { 
             id
             firstName
         }
      }`,
      then: [
          {
              type: "User",
              url: location2,
              insertionPoint: ["allUsers"],
              selection: `{
                  lastName
              }`
          }
      ]
  },
]
```
