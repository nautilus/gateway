package gateway

import (
	"bytes"
	"encoding/gob"
)

// QueryPersister archives and retrieves query plans
type QueryPersister interface {
	// PersistQuery saves the body of the query somewhere
	// and returns an address that it can be retrieved later
	PersistQuery(plan *QueryPlan) (string, error)

	// RestoreQuery takes an address and returns the referenced query plan (if it exists)
	// if no query plan exists, an error is returned
	RestoreQuery(address string) (*QueryPlan, error)
}

// ContentAddressPersister uses a byte representation of the query plan as its addresses
// removing the need for a centralized storage. This does not allow for any kind of list of
// allowed queries since the address is assumed to be valid if it contains a QueryPlan.
type ContentAddressPersister struct{}

// PersistQuery saves the query plan in the in-memory map
func (p *ContentAddressPersister) PersistQuery(plan *QueryPlan) (string, error) {
	// a place to write the result
	var address bytes.Buffer

	err := gob.NewEncoder(&address).Encode(plan)
	if err != nil {
		return "", err
	}

	return address.String(), nil
}

// RestoreQuery retrieves the plan referenced by the provided address
func (p *ContentAddressPersister) RestoreQuery(address string) (*QueryPlan, error) {
	return nil, nil
}


func init () {
	gob.Register(graphql.QueryerFunc)
}