package gateway

import (
	"errors"
	"fmt"
	"sync"

	"github.com/vektah/gqlparser/ast"
)

// JSONObject is a typdef for map[string]interface{} to make structuring json responses easier.
type JSONObject map[string]interface{}

// Executor is responsible for executing a query plan against the remote
// schemas and returning the result
type Executor interface {
	Execute(*QueryPlan) (JSONObject, error)
}

// ParallelExecutor executes the given query plan by starting at the root of the plan and
// walking down the path stitching the results together
type ParallelExecutor struct{}

type queryExecutionResult struct {
	InsertionPoint []string
	ParentType     string
	Result         JSONObject
}

// Execute returns the result of the query plan
func (executor *ParallelExecutor) Execute(plan *QueryPlan) (JSONObject, error) {
	// a place to store the result
	result := JSONObject{}

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
	go executeStep(plan.RootStep, plan.RootStep.InsertionPoint, resultCh, errCh, stepWg)

	// start a goroutine to add results to the list
	go func() {
		for {
			select {
			// we have a new result
			case payload := <-resultCh:
				log.Debug("Received result to be inserted at ", payload.InsertionPoint)

				// if there is a deep insertion point
				if len(payload.InsertionPoint) > 1 {
					path := payload.InsertionPoint[:len(payload.InsertionPoint)-1]
					key := payload.InsertionPoint[len(payload.InsertionPoint)-1]

					return
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
					objMap, ok := obj.(JSONObject)
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
						nodeMap, ok := nodeValue.(JSONObject)
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

						// use that value in the right spot
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

func executeStep(step *QueryPlanStep, insertionPoint []string, resultCh chan queryExecutionResult, errCh chan error, stepWg *sync.WaitGroup) {
	// each selection set that is the parent of another query must ask for the id
	for _, nextStep := range step.Then {
		// the next query will go
		path := nextStep.InsertionPoint[:len(nextStep.InsertionPoint)-1]

		// the selection set we need to add `id` to
		target := step.SelectionSet
		var targetField *ast.Field

		for _, point := range path {
			// look for the selection with that name
			for _, selection := range applyDirectives(target) {
				// if we still have to walk down the selection but we found the right branch
				if selection.Name == point {
					target = selection.SelectionSet
					targetField = selection
					break
				}
			}
		}

		// if we couldn't find the target
		if target == nil {
			errCh <- fmt.Errorf("Could not find field to add id to: %v", path)
			return
		}

		// if the target does not currently ask for id we need to add it
		addID := true
		for _, selection := range applyDirectives(target) {
			if selection.Name == "id" {
				addID = false
				break
			}
		}

		// add the ID to the selection set if necessary
		if addID {
			target = append(target, &ast.Field{
				Name: "id",
			})
		}

		// make sure the selection set contains the id
		targetField.SelectionSet = target
	}

	// log the query
	log.QueryPlanStep(step)

	// TODO: using the insertion point, find the id of the object we are resolving this
	// step for

	// generate the query that we have to send for this step
	query := buildQueryForExecution(step.ParentType, step.SelectionSet)

	// execute the query
	queryResult, err := step.Queryer.Query(query)
	if err != nil {
		errCh <- err
	}

	// NOTE: this insertion point could point to a list of values. If it did, we have to have
	//       passed it to the this invocation of this function. It is safe to trust this
	//       InsertionPoint as the right place to insert this result.

	// send the result to be stitched in with our accumulator
	resultCh <- queryExecutionResult{
		InsertionPoint: step.InsertionPoint,
		Result:         queryResult,
		ParentType:     step.ParentType,
	}

	// if there are next steps
	if len(step.Then) > 0 {
		// we need to find the ids of the objects we are inserting into and then kick of the worker with the right
		// insertion point. For lists, insertion points look like: ["user", "friends:0", "catPhotos:0", "owner"]
		for _, dependent := range step.Then {
			insertPoints, err := findInsertionPoints(step.InsertionPoint, step.SelectionSet, queryResult, [][]string{step.InsertionPoint})
			if err != nil {
				errCh <- err
			}

			// this dependent needs to fire for every object that the insertion point references
			for _, insertionPoint := range insertPoints {
				fmt.Println(insertionPoint)
				// stepWg.Add(1)
				// go executeStep(dependent, dependent.InsertionPoint, resultCh, errCh, stepWg)
			}

			// look up the id of the object we are inserting into
			fmt.Println(dependent.InsertionPoint)
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// findInsertionPoints returns the list of insertion points where this step should be executed.
func findInsertionPoints(targetPoints []string, selectionSet ast.SelectionSet, result JSONObject, startingPoints [][]string) ([][]string, error) {
	oldBranch := startingPoints
	for _, branch := range oldBranch {
		if len(branch) > 1 {
			branch = branch[:max(len(branch), 1)]
		}
	}

	// track the root of the selection set while  we walk
	selectionSetRoot := selectionSet

	// a place to refer to parts of the results
	resultChunk := result

	log.Debug("Starting with ", oldBranch)

	// if our starting point is []string{"users:0", "photoGallery"} then we know everything up until photoGallery
	// is along the path of the steps insertion point
	for pointI := len(oldBranch[0]); pointI < len(targetPoints); pointI++ {
		// the point in the steps insertion path that we want to add
		point := targetPoints[pointI]

		log.Debug("Looking for ", point)

		// if we are at the last field, just add it
		if pointI == len(targetPoints)-1 {
			log.Debug("Pushing final point on ends ", point)
			for i, points := range oldBranch {
				oldBranch[i] = append(points, point)
			}
		} else {
			// there should be a field in the root selection set that has the target point
			for _, selection := range applyDirectives(selectionSetRoot) {
				// if the selection has the right name we need to add it to the list
				if selection.Alias == point || selection.Name == point {
					// make sure we are looking at the top of the selection set next time
					selectionSetRoot = selection.SelectionSet

					// the bit of result chunk with the appropriate key should be a list
					rootValue, ok := resultChunk[point]
					if !ok {
						return nil, errors.New("Root value of result chunk could not be found")
					}

					log.Debug("Found selection for ", point)
					// get the type of the object in question
					selectionType := selection.Definition.Type

					// if the type is a list
					if selectionType.Elem != nil {
						log.Debug("Selection is a list")
						log.Debug(resultChunk)

						// make sure the root value is a list
						rootList, ok := rootValue.([]JSONObject)
						if !ok {
							return nil, errors.New("Root value of result chunk was not a list")
						}

						// build up a new list of insertion points
						newInsertionPoints := [][]string{}

						// each value in this list contributes an insertion point
						for entryI, resultEntry := range rootList {

							newBranchSet := make([][]string, len(oldBranch))
							copy(newBranchSet, oldBranch)
							log.Debug("previous list before adding list branch -> ", newBranchSet)

							// add the path to the end of this for the entry we just added
							for i, newBranch := range newBranchSet {
								// the point we are going to add to the list
								entryPoint := fmt.Sprintf("%s:%v", selection.Name, entryI)
								// if we are looking at the second to last thing in the insertion list
								if pointI == len(targetPoints)-2 {
									// look for an id
									id, ok := resultEntry["id"]
									if !ok {
										return nil, errors.New("Could not find the id for elements in target list")
									}

									// add the id to the entry so that the executor can use it to form its query
									entryPoint = fmt.Sprintf("%s#%v", entryPoint, id)

									fmt.Println("FINAL", point, entryPoint, id)
								}

								log.Debug("Adding ", entryPoint, " to list")
								newBranchSet[i] = append(newBranch, entryPoint)
							}

							// compute the insertion points for that entry
							entryInsertionPoints, err := findInsertionPoints(targetPoints, selectionSetRoot, resultEntry, newBranchSet)
							if err != nil {
								return nil, err
							}

							for _, point := range entryInsertionPoints {
								// add the list of insertion points to the acumulator
								newInsertionPoints = append(newInsertionPoints, point)
							}
						}

						// return the flat list of insertion points created by our children
						return newInsertionPoints, nil
					}

					// we are encountering something that isn't a list so it must be an object or a scalar
					// regardless, we just need to add the point to the end of each list
					for i, points := range oldBranch {
						oldBranch[i] = append(points, point)
					}

					if pointI == len(targetPoints)-2 {
						// the final entry is an object so we need to add the id to the point
						for i := range oldBranch {
							// make sure the root value is a list
							rootObj, ok := rootValue.(JSONObject)
							if !ok {
								return nil, errors.New("Root value of result chunk was not an object")

							}

							// look up the id of the object
							id, ok := rootObj["id"]
							if !ok {
								return nil, errors.New("Could not find the id for the object")
							}

							oldBranch[i][pointI] = fmt.Sprintf("%s#%v", oldBranch[i][pointI], id)
						}
					}

					// we're done looking through the selection set
					continue
				}

			}
		}
	}

	// return the aggregation
	return oldBranch, nil
}

func buildQueryForExecution(objectType string, selectionSet ast.SelectionSet) *ast.QueryDocument {
	return nil
}
