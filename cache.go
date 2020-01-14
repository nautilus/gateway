package gateway

import (
	"errors"
	"sync"
	"time"

	"crypto/sha256"
	"encoding/hex"
)

// In general, "query persistance" is a term for a family of optimizations that involve
// storing some kind of representation of the queries that the client will send. For
// nautilus, this allows for the pre-computation of query plans and can drastically speed
// up response times.
//
// There are a few different strategies when it comes to timing the computation of these
// plans. Each strategy has its own trade-offs and should be carefully considered
//
// Automatic Persisted Queries:
// 		- client asks for the query associated with a particular hash
// 		- if the server knows that hash, execute the query plan. if not, return with a known value
//		- if the client sees the known value, resend the query with the full query body
// 		- the server will then calculate the plan and save it for later use
//      - if the client sends a known hash along with the query body, the query body is ignored
//
//      pros/cons:
//		- no need for a build step
// 		- the client can send any queries they want
//
// StaticPersistedQueries (not implemented here):
//		- as part of a build step, the gateway is given the list of queries and associated
//			hashes
//		- the client only sends the hash with queries
// 		- if the server recognizes the hash, execute the query. Otherwise, return with en error
//
//		pros/cons:
//		- need for a separate build step that prepares the queries and shares it with the server
//		- tighter control on operations. The client can only send queries that are approved (pre-computed)

// MessageMissingCachedQuery is the string that the server sends when the user assumes that the server knows about
// a caches query plan
const MessageMissingCachedQuery = "PersistedQueryNotFound"

// QueryPlanCache decides when to compute a plan
type QueryPlanCache interface {
	Retrieve(ctx *PlanningContext, hash *string, planner QueryPlanner) (QueryPlanList, error)
}

// WithNoQueryPlanCache is the default option and disables any persisted query behavior
func WithNoQueryPlanCache() Option {
	return WithQueryPlanCache(&NoQueryPlanCache{})
}

// NoQueryPlanCache will always compute the plan for a query, regardless of the value passed as `hash`
type NoQueryPlanCache struct{}

// Retrieve just computes the query plan
func (p *NoQueryPlanCache) Retrieve(ctx *PlanningContext, hash *string, planner QueryPlanner) (QueryPlanList, error) {
	return planner.Plan(ctx)
}

// WithQueryPlanCache sets the query plan cache that the gateway will use
func WithQueryPlanCache(p QueryPlanCache) Option {
	return func(g *Gateway) {
		g.queryPlanCache = p
	}
}

// WithAutomaticQueryPlanCache enables the "automatic persisted query" technique
func WithAutomaticQueryPlanCache() Option {
	return WithQueryPlanCache(NewAutomaticQueryPlanCache())
}

type queryPlanCacheItem struct {
	LastUsed time.Time
	Value    QueryPlanList
}

// AutomaticQueryPlanCache is a QueryPlanCache that will use the hash if it points to a known query plan,
// otherwise it will compute the plan and save it for later, to be referenced by the designated hash.
type AutomaticQueryPlanCache struct {
	cache map[string]*queryPlanCacheItem
	ttl   time.Duration
	// the automatic query plan cache needs to clear itself of query plans that have been used
	// recently. This coordination requires a channel over which events can be trigger whenever
	// a query is fired, triggering a check to clean up other queries.
	retrievedPlan chan bool
	// a boolean to track if there is a timer that needs to be reset
	resetTimer bool
	// a mutex on the timer bool
	timeMutex sync.Mutex
}

// WithCacheTTL updates and returns the cache with the new cache lifetime. Queries that haven't been
// used in that long are cleaned up on the next query.
func (c *AutomaticQueryPlanCache) WithCacheTTL(duration time.Duration) *AutomaticQueryPlanCache {
	return &AutomaticQueryPlanCache{
		cache:         c.cache,
		ttl:           duration,
		retrievedPlan: c.retrievedPlan,
		resetTimer:    c.resetTimer,
	}
}

// NewAutomaticQueryPlanCache returns a fresh instance of
func NewAutomaticQueryPlanCache() *AutomaticQueryPlanCache {
	return &AutomaticQueryPlanCache{
		cache: map[string]*queryPlanCacheItem{},
		// default cache lifetime of 3 days
		ttl:           10 * 24 * time.Hour,
		retrievedPlan: make(chan bool),
		resetTimer:    false,
	}
}

// Retrieve follows the "automatic query persistance" technique. If the hash is known, it will use the referenced query plan.
// If the hash is not know but the query is provided, it will compute the plan, return it, and save it for later use.
// If the hash is not known and the query is not provided, it will return with an error prompting the client to provide the hash and query
func (c *AutomaticQueryPlanCache) Retrieve(ctx *PlanningContext, hash *string, planner QueryPlanner) (QueryPlanList, error) {

	// when we're done with retrieving the value we have to clear the cache
	defer func() {
		// spawn a goroutine that might be responsible for clearing the cache
		go func() {
			// check if there is a timer to reset
			c.timeMutex.Lock()
			resetTimer := c.resetTimer
			c.timeMutex.Unlock()

			// if there is already a goroutine that's waiting to clean things up
			if resetTimer {
				// just reset their time
				c.retrievedPlan <- true
				// and we're done
				return
			}
			c.timeMutex.Lock()
			c.resetTimer = true
			c.timeMutex.Unlock()

			// otherwise this is the goroutine responsible for cleaning up the cache
			timer := time.NewTimer(c.ttl)

			// we will have to consume more than one input
		TRUE_LOOP:
			for {
				select {
				// if another plan was retrieved
				case <-c.retrievedPlan:
					// reset the time
					timer.Reset(c.ttl)

				// if the timer dinged
				case <-timer.C:
					// there is no longer a timer to reset
					c.timeMutex.Lock()
					c.resetTimer = false
					c.timeMutex.Unlock()

					// loop over every time in the cache
					for key, cacheItem := range c.cache {
						// if the cached query hasn't been used recently enough
						if cacheItem.LastUsed.Before(time.Now().Add(-c.ttl)) {
							// delete it from the cache
							delete(c.cache, key)
						}
					}

					// stop consuming
					break TRUE_LOOP
				}
			}

		}()
	}()

	// if we have a cached value for the hash
	if cached, hasCachedValue := c.cache[*hash]; hasCachedValue {
		// update the last used
		cached.LastUsed = time.Now()
		// return it
		return cached.Value, nil
	}

	// we dont have a cached value

	// if we were not given a query string
	if ctx.Query == "" {
		// return an error with the magic string
		return nil, errors.New(MessageMissingCachedQuery)
	}

	// compute the plan
	plan, err := planner.Plan(ctx)
	if err != nil {
		return nil, err
	}

	// if there is no hash
	if *hash == "" {
		hashString := sha256.Sum256([]byte(ctx.Query))
		// generate a hash that will identify the query for later use
		*hash = hex.EncodeToString(hashString[:])
	}

	// save it for later
	c.cache[*hash] = &queryPlanCacheItem{
		LastUsed: time.Now(),
		Value:    plan,
	}

	// we're done
	return plan, nil
}
