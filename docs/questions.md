Does it make sense to share the resolution of certain fields?
  - for fields on Query, probably not.
  - for fields on Mutation, it could be an interesting pattern 
    - use mutations as events of an application?
  - for fields on the generic object types
    - seems like "shared state" and could be indication of a bad domain separation
    - it has to be the case for `id` but that could be a necessary exception
