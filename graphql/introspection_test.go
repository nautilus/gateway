package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntrospectQuery_savesQueryType(t *testing.T) {
	schema, err := IntrospectAPI(&MockQueryer{
		JSONObject{
			"__schema": JSONObject{
				"queryType": JSONObject{
					"name": "Query",
				},
			},
		},
	})

	if err != nil {
		t.Error(err.Error())
		return
	}

	if schema == nil {
		t.Error("Received nil schema")
		return
	}

	assert.Equal(t, "Query", schema.Query.Name)
}
