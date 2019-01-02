package gateway

import (
	"fmt"
	"sync"

	"github.com/vektah/gqlparser/ast"
)

// Executor is responsible for executing a query plan against the remote
// schemas and returning the result
type Executor interface {
	Execute(*QueryPlan) (map[string]interface{}, error)
}

// ParallelExecutor executes the given query plan by starting at the root of the plan and
// walking down the path stitching the results together
type ParallelExecutor struct{}

type queryExecutionResult struct {
	InsertionPoint []string
	ParentType     string
	Result         map[string]interface{}
}

// Execute returns the result of the query plan
func (executor *ParallelExecutor) Execute(plan *QueryPlan) (map[string]interface{}, error) {
	// a place to store the result
	result := map[string]interface{}{}

	// a channel to recieve query results
	resultCh := make(chan queryExecutionResult, 10)
	defer close(resultCh)

	// a wait group so we know when we're done with all of the steps
	stepWg := &sync.WaitGroup{}

	// and a channel for errors
	errCh := make(chan error, 10)
	defer close(errCh)

	// a channel to close the goroutine
	closeCh := make(chan bool)
	defer close(closeCh)

	// we need to start at the root strep
	stepWg.Add(1)
	go executeStep(plan.RootStep, resultCh, errCh, stepWg)

	// start a goroutine to add results to the list
	go func() {
		for {
			select {
			// we have a new result
			case payload := <-resultCh:
				log.Debug("New result ", payload.InsertionPoint)

				// if there is a deep insertion point
				if len(payload.InsertionPoint) > 1 {
					path := payload.InsertionPoint[:len(payload.InsertionPoint)-1]
					key := payload.InsertionPoint[len(payload.InsertionPoint)-1]

					// the object we are accessing
					var obj interface{}

					// find the object indicated by the path
					for _, point := range path {
						value, ok := result[point]
						if !ok {
							errCh <- fmt.Errorf("Could not find value to insert: %v", payload.InsertionPoint)
							return
						}
						// reassign the value
						obj = value
					}

					// make sure its a real object
					objMap, ok := obj.(map[string]interface{})
					if !ok {
						errCh <- fmt.Errorf("Could not find value to insert: %v", payload.InsertionPoint)
						return
					}

					// assign the result of the query to the final result
					objMap[key] = payload.Result

					// if we are inserting something other than a top level query
					if payload.ParentType != "Query" {
						// look up the node field
						nodeValue, ok := payload.Result["node"]
						if !ok {
							errCh <- fmt.Errorf("Could not find node")
							return
						}
						nodeMap, ok := nodeValue.(map[string]interface{})
						if !ok {
							errCh <- fmt.Errorf("Could not find node")
							return
						}

						// grab the field underneath node that we care about to do the stitching
						realValue, ok := nodeMap[key]
						if !ok {
							errCh <- fmt.Errorf("Could not find %s field under node", key)
							return
						}

						objMap[key] = realValue
					}

				} else {
					// there isn't a deep insertion point so we can just merge the result with our accumulator
					for key, value := range payload.Result {
						result[key] = value
					}
				}

				// one of the queries is done
				stepWg.Done()

			// we're done
			case <-closeCh:
				return
			}
		}
	}()

	// there are 2 possible options:
	// - either the wait group finishes
	// - we get a messsage over the error chan

	// in order to wait for either, let's spawn a go routine
	// that waits until all of the steps are built and notifies us when its done
	doneCh := make(chan bool)
	defer close(doneCh)

	go func() {
		// when the wait group is finished
		stepWg.Wait()
		// push a value over the channel
		doneCh <- true
	}()

	// wait for either the error channel or done channel
	select {
	// there was an error
	case err := <-errCh:
		log.Warn(fmt.Sprintf("Ran into execution error: %s", err.Error()))
		closeCh <- true
		// bubble the error up
		return nil, err
	// we are done
	case <-doneCh:
		closeCh <- true
		// we're done here
		return result, nil
	}

}

func executeStep(step *QueryPlanStep, resultCh chan queryExecutionResult, errCh chan error, stepWg *sync.WaitGroup) {

	// generate the query that we have to send for this step
	query := buildQueryForExecution(step.ParentType, step.SelectionSet)

	// execute the query
	queryResult, err := step.Queryer.Query(query)
	if err != nil {
		errCh <- err
	}

	// send the result to be stitched
	resultCh <- queryExecutionResult{
		InsertionPoint: step.InsertionPoint,
		Result:         queryResult,
		ParentType:     step.ParentType,
	}

	// kick off any dependencies
	for _, dependent := range step.Then {
		stepWg.Add(1)
		go executeStep(dependent, resultCh, errCh, stepWg)
	}

}

func buildQueryForExecution(objectType string, selectionSet ast.SelectionSet) *ast.QueryDocument {
	return nil
}
