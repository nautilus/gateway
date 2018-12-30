## Query Planning

Given schema1 located at `location1`:
```graphql
type Query { 
    lastName:String!
}

type Query {
    allUsers: [User!]!
}
```

And `schema2` located at `location2`:
```graphql
type User { 
    firstName: String!
}
```

A query that looks like:
```gql
query { 
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
      url: location1,
      query: gql`
        {
           allUsers { 
               id
               firstName
           }
        }
      `,
      then: [
          // an entry for each user
          {
              url: location2,
              query: gql`{
                  node(id: "1234") {
                      lastName
                  }
              }`
          }
      ]
  },
]
```
