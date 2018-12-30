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

