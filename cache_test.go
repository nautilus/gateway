package gateway

import (
	"errors"
	"testing"
	"time"

	"github.com/nautilus/graphql"
	"github.com/stretchr/testify/assert"
)

// MockPlanner always returns the provided list of plans. Useful in testing.
type testPlannerCounter struct {
	Count int
	Plans QueryPlanList
}

func (p *testPlannerCounter) Plan(*PlanningContext) (QueryPlanList, error) {
	// increment the count
	p.Count++

	// return the plans
	return p.Plans, nil
}

func TestCacheOptions(t *testing.T) {
	// turn the combo into a remote schema
	schema, _ := graphql.LoadSchema(`
		type Query {
			value: String!
		}
	`)

	// create a gateway that doesn't wrap any schemas and has no query plan cache
	gw, err := New([]*graphql.RemoteSchema{
		{
			URL:    "asdf",
			Schema: schema,
		},
	}, WithNoQueryPlanCache())
	if !assert.Nil(t, err) {
		return
	}

	// make sure that the query plan cache is one that doesn't cache
	_, ok := gw.queryPlanCache.(*NoQueryPlanCache)
	assert.True(t, ok)
}

func TestNoQueryPlanCache(t *testing.T) {
	cacheKey := "asdf"
	// the plan we are expecting back
	plans := QueryPlanList{}
	// instantiate a planner that can count how many times it was invoked
	planner := &testPlannerCounter{
		Plans: plans,
	}

	// an instance of the NoCache cache
	cache := &NoQueryPlanCache{}

	// ask the cache to retrieve the same hash twice
	plan1, err := cache.Retrieve(nil, &cacheKey, planner)
	if !assert.Nil(t, err) {
		return
	}
	plan2, err := cache.Retrieve(nil, &cacheKey, planner)
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
	cacheKey := "asdf"
	// the plan we are expecting back
	plans := QueryPlanList{}
	// instantiate a planner that can count how many times it was invoked
	planner := &testPlannerCounter{
		Plans: plans,
	}

	// an instance of the NoCache cache
	cache := NewAutomaticQueryPlanCache()

	// passing no query and an unknown hash should return an error with the magic string
	plan1, err := cache.Retrieve(&PlanningContext{}, &cacheKey, planner)
	if !assert.NotNil(t, err, "error was nil") {
		return
	}
	assert.Equal(t, err.Error(), MessageMissingCachedQuery)
	assert.Nil(t, plan1)

	// passing a non-empty query along with a hash associates the resulting plan with the hash
	plan2, err := cache.Retrieve(&PlanningContext{Query: "hello"}, &cacheKey, planner)
	if !assert.Nil(t, err) {
		return
	}
	assert.Equal(t, plan2, plans)

	// do the same thing we did in step 1 (ask without a query body)
	plan3, err := cache.Retrieve(&PlanningContext{}, &cacheKey, planner)
	assert.Equal(t, plan3, plans)
	if !assert.Nil(t, err) {
		return
	}

	// we should have only computed the plan once
	assert.Equal(t, 1, planner.Count)
}

func TestAutomaticQueryPlanCache_passPlannerErrors(t *testing.T) {
	cacheKey := "asdf"
	// instantiate a planner that can count how many times it was invoked
	planner := &MockErrPlanner{errors.New("Error")}

	// an instance of the NoCache cache
	cache := NewAutomaticQueryPlanCache()

	// passing no query and an unknown hash should return an error with the magic string
	_, err := cache.Retrieve(&PlanningContext{Query: "Asdf"}, &cacheKey, planner)
	if !assert.NotNil(t, err, "error was nil") {
		return
	}
}

func TestAutomaticQueryPlanCache_setCacheKey(t *testing.T) {
	// instantiate a planner that can count how many times it was invoked
	planner := &testPlannerCounter{
		Plans: QueryPlanList{},
	}

	// an instance of the NoCache cache
	cache := NewAutomaticQueryPlanCache()

	// the key of the cache
	cacheKey := ""

	// plan a query
	cache.Retrieve(&PlanningContext{Query: "hello"}, &cacheKey, planner)

	// make sure that the key was changed
	if cacheKey == "" {
		t.Error("Cache key was not updated")
		return
	}
}

func TestAutomaticQueryPlanCache_garbageCollection(t *testing.T) {
	cacheKey := "asdf"
	// the plan we are expecting back
	plans := QueryPlanList{}
	// instantiate a planner that can count how many times it was invoked
	planner := &testPlannerCounter{
		Plans: plans,
	}

	// an instance of the NoCache cache
	cache := NewAutomaticQueryPlanCache().WithCacheTTL(100 * time.Millisecond)

	// retrieving the plan back to back should hit the cached version
	_, err := cache.Retrieve(&PlanningContext{Query: "hello"}, &cacheKey, planner)
	if !assert.Nil(t, err) {
		return
	}
	_, err = cache.Retrieve(&PlanningContext{Query: "hello"}, &cacheKey, planner)
	if !assert.Nil(t, err) {
		return
	}
	// the plan should have only been computed once
	assert.Equal(t, 1, planner.Count)

	// wait longer than the cache ttl
	time.Sleep(150 * time.Millisecond)

	// ask for it twice more
	_, err = cache.Retrieve(&PlanningContext{Query: "hello"}, &cacheKey, planner)
	if !assert.Nil(t, err) {
		return
	}
	_, err = cache.Retrieve(&PlanningContext{}, &cacheKey, planner)
	if !assert.Nil(t, err) {
		return
	}

	// we should have only generated the plan twice now (once more than before)
	assert.Equal(t, 2, planner.Count)
}
