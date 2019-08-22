package gateway

// In general, "query persistance" is a term for a family of optimizations that involves
// storing some kind of representation of the queries that the client will send. For
// nautilus, this allows for the pre-computation of query plans and can drastically speed
// up response times.
//
// There are different strategies when it comes to timing the computation of these
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
//
// StaticPersistedQueries (not implemented yet):
//		- as part of a build step, the gateway is given the list of queries and associated
//			hashes
//		- the client only sends the hash with queries
// 		- if the server recognizes the hash, execute the query. Otherwise, return with en error
//
//		pros/cons:
//		- need for a separate build step that prepares the queries and shares it with the server
//		- tighter control on operations. The client can only send queries that are approved (pre-computed)

// QueryPersister decides when to compute a plan
type QueryPersister interface {
	Persist(ctx PlanningContext, hash string, planner QueryPlanner) ([]*QueryPlan, error)
}

// WithNoPersistedQueries is the default option and disables any persisted query behavior
func WithNoPersistedQueries() Option {
	return func(g *Gateway) {

	}
}

// NoQueryPersister will always compute the plan for a query, regardless of the value passed as `hash`
type NoQueryPersister struct{}

// Persist just computes the query plan
func (p *NoQueryPersister) Persist(ctx *PlanningContext, hash string, planner QueryPlanner) ([]*QueryPlan, error) {
	return planner.Plan(ctx)
}

// WithAutomaticPersistedQueries enables the "automatic persisted query" technique
func WithAutomaticPersistedQueries() Option {
	return func(g *Gateway) {

	}
}
