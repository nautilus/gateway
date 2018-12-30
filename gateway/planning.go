package gateway

import "github.com/vektah/gqlparser/ast"

// QueryPlanStep represents a step in the plan required to fulfill a query.
type QueryPlanStep struct {
	Location string
	Query    *ast.QueryDocument
}

// QueryPlan is the full plan to resolve a particular query
type QueryPlan []QueryPlanStep

// QueryPlanner is responsible for taking a parsed graphql string, and returning the steps to
// execute that fulfill the response
type QueryPlanner interface {
	Plan(string, FieldLocationMap) (*QueryPlan, error)
}

// NaiveQueryPlanner does the most basic level of query planning
type NaiveQueryPlanner struct{}

// Plan computes the query plan required to fulfill the provided query
func (p *NaiveQueryPlanner) Plan(query string, locations FieldLocationMap) (*QueryPlan, error) {
	return nil, nil
}
