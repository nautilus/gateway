package graphql

import (
	"testing"
)

func TestLoadSchema_succeed(t *testing.T) {
	// load the schema string
	schema, _ := LoadSchema(`
		type Query {
			foo: String
		}
	`)

	_, ok := schema.Types["Query"]
	if !ok {
		t.Error("Could not find Query type")
	}
}
func TestLoadSchema_fails(t *testing.T) {
	// load the schema string
	_, err := LoadSchema(`
		type Query a {
			foo String
		}
	`)
	if err == nil {
		t.Error("Did not encounter error when type def had errors")
		return
	}
}
