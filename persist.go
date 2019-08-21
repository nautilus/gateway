package gateway

// QueryPersister archives and retrieves query plans
type QueryPersister interface {
	// PersistQuery saves the body of the query somewhere
	// and returns an address that it can be retrieved later
	PersistQuery(plan *QueryPlan) (string, error)

	// RestoreQuery takes an address and returns the referenced query plan (if it exists)
	// if no query plan exists, an error is returned
	RestoreQuery(address string) (*QueryPlan, error)

	// Hydrate is called when the gateway first starts and can be used to pay any up-front costs
	// associated with grabbing the list of persisted queries
	Hydrate() error
}

// InMemoryQueryPersister saves the provided plans in an in-memory structure
type InMemoryQueryPersister struct {
	queries map[string]*QueryPlan
}

// Hydrate doesn't do anything in this persister
func (p *InMemoryQueryPersister) Hydrate() error {
	return nil
}

// PersistQuery saves the query plan in the in-memory map
func (p *InMemoryQueryPersister) PersistQuery(plan *QueryPlan) (string, error) {

	return "", nil
}
