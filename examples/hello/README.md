# Hello World

This example is meant to act as a simple proof of concept and has the bare minimum
needed to showcase a distributed GraphQL API. Since there is no need for any custom
logic, this example relies on the gateway cli to run.

## Running the Example

- Start both services by running `go run <filename>` in 2 separate terminals.
- Start the gateway over those 2 services: `graphql-gateway start --services http://localhost:8080,http://localhost:8081`
