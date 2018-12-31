package gateway

// Executor is responsible for executing a query plan against the remote
// schemas and returning the result
type Executor interface {
	Execute(*QueryPlan) map[interface{}]interface{}
}

// SerialExecutor executes the query plan without worrying about which network requests
// can be parallelized
type SerialExecutor struct{}

// Execute returns the result of the query plan
func (executor *SerialExecutor) Execute(plan *QueryPlan) map[interface{}]interface{} {
	return map[interface{}]interface{}{}
}
