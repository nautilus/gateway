# graphql-gateway
[![Build Status](https://travis-ci.com/AlecAivazis/graphql-gateway.svg?branch=master)](https://travis-ci.com/AlecAivazis/graphql-gateway)

An api gateway for graphql services

# Motivation

* Schema stitching is great to keep domains separate, but makes it hard to build one conhesive API that consolidates
the schemas of various backend services. 

* Instead of treating types as if they belong to a single domain, certain 
types can be thought of as living on the boundary of domains and can be "owned" by multiple domains.

## Boundary Types

# Thanks

Thanks to Martijn Walraven for giving a talk at graphql-summit which inspired this project.
