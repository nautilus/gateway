package gateway

// Executor is responsible for executing a query plan against the remote
// schemas and returning the result
type Executor interface {
	Execute(*QueryPlan) map[interface{}]interface{}
}

// Notes:
// 	- Don't have to worry about aliases. The backend server will handle the renaming
//	- Planner currently doesn't provide steps with multiple dependencies

// SerialExecutor executes the query plan without worrying about which network requests
// can be parallelized
type SerialExecutor struct{}

// Execute returns the result of the query plan
func (executor *SerialExecutor) Execute(plan *QueryPlan) map[interface{}]interface{} {
	return map[interface{}]interface{}{}
}
