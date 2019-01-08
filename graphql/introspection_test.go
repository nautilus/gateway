package graphql

import (
	"testing"
)

func TestIntrospectQuery_savesQueryType(t *testing.T) {
	_, err := IntrospectAPI(&MockQueryer{
		IntrospectionQueryResult{},
	})

	if err != nil {
		t.Error(err.Error())
		return
	}
}
