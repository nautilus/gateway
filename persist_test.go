package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockPlanner always returns the provided list of plans. Useful in testing.
type testPlannerCounter struct {
	Count int
	Plans []*QueryPlan
}

func (p *testPlannerCounter) Plan(*PlanningContext) ([]*QueryPlan, error) {
	// increment the count
	p.Count++

	// return the plans
	return p.Plans, nil
}

func TestNoQueryPlanCache(t *testing.T) {
	// the plan we are expecting back
	plans := []*QueryPlan{}
	// instantiate a planner that can count how many times it was invoked
	planner := &testPlannerCounter{
		Plans: plans,
	}

	// an instance of the NoCache cache
	cache := &NoQueryPlanCache{}

	// ask the cache to retrieve the same hash twice
	plan1, err := cache.Retrieve(nil, "asdf", planner)
	if !assert.Nil(t, err) {
		return
	}
	plan2, err := cache.Retrieve(nil, "asdf", planner)
	if !assert.Nil(t, err) {
		return
	}

	// make sure that we computed two plans
	assert.Equal(t, 2, planner.Count)
	// and we got the same plan back both times
	assert.Equal(t, plan1, plans)
	assert.Equal(t, plan2, plans)
}

func TestAutomaticQueryPlanCache(t *testing.T) {
	// the plan we are expecting back
	plans := []*QueryPlan{}
	// instantiate a planner that can count how many times it was invoked
	planner := &testPlannerCounter{
		Plans: plans,
	}

	// an instance of the NoCache cache
	cache := &AutomaticQueryPlanCache{}

	// passing no query and an unknown hash should return an error with the magic string
	plan1, err := cache.Retrieve(&PlanningContext{}, "asdf", planner)
	if !assert.NotNil(t, err, "error was nil") {
		return
	}
	assert.Equal(t, err.Error(), MessageMissingCachedQuery)
	assert.Equal(t, plan1, plans)

	// passing a non-empty query along with a hash associates the resulting plan with the hash
	plan2, err := cache.Retrieve(&PlanningContext{Query: "hello"}, "asdf", planner)
	if !assert.Nil(t, err, err.Error()) {
		return
	}
	assert.Equal(t, plan2, plans)

	// do the same thing we did in step 1 (ask without a query body)
	plan3, err := cache.Retrieve(&PlanningContext{}, "asdf", planner)
	assert.Equal(t, plan3, plans)
	if !assert.Nil(t, err, err.Error()) {
		return
	}

	// we should have only computed the plan once
	assert.Equal(t, 1, planner.Count)
}
