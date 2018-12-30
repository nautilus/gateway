package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/ast"
)

func TestPlanQuery_singleRootField(t *testing.T) {
	// the location for the schema
	location := "url1"

	// the location map for fields for this query
	locations := FieldLocationMap{}
	locations.RegisterLocation("Query", "allUsers", location)
	locations.RegisterLocation("User", "firstName", location)

	// compute the plan for a query that just hits one service
	plan, err := (&NaiveQueryPlanner{}).Plan(`
		{
			allUsers {
				firstName
			}
		}
	`, locations)
	// if something went wrong planning the query
	if err != nil {
		// the test is over
		t.Errorf("encountered error when building schema: %s", err.Error())
		return
	}

	// make sure we got a plan
	if plan == nil {
		t.Error("generated a nil plan for a query")
		return
	}

	// make sure the plan matches our expectation

	// there is only one step
	assert.Len(t, plan, 1)
	firstStep := (*plan)[0]

	// make sure the query was going to the location
	assert.Equal(t, location, firstStep.Location)

	// make sure the query of the step matches our expectations
	targetQuery, _ := parseQuery(`
		query {
			allUsers { firstName }
		}
	`)
	assert.Equal(t, ast.Dump(targetQuery), ast.Dump(firstStep.Query))

}

// func TestPlanQuery_multipleRootFields(t *testing.T) {
// 	t.Error("Not implemented")
// }

// func TestPlanQuery_mutationsInSeries(t *testing.T) {
// 	t.Error("Not implemented")
// }

// func TestPlanQuery_subGraphs(t *testing.T) {
// 	t.Error("Not implemented")
// }

// func TestPlanQuery_siblingFields(t *testing.T) {
// 	t.Error("Not implemented")
// }

// func TestPlanQuery_duplicateFieldsOnEither(t *testing.T){
// make sure that if I have the same field defined on both schemas we dont create extraneous calls
// }

// func TestPlanQuery_groupsConflictingFields(t *testing.T) {
// if I can find a field in 4 different services, look for the one I"m already going to
// }
