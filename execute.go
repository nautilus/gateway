package gateway

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/nautilus/graphql"
	"github.com/vektah/gqlparser/ast"
)

// Executor is responsible for executing a query plan against the remote
// schemas and returning the result
type Executor interface {
	Execute(ctx *ExecutionContext) (map[string]interface{}, error)
}

// ParallelExecutor executes the given query plan by starting at the root of the plan and
// walking down the path stitching the results together
type ParallelExecutor struct{}

type queryExecutionResult struct {
	InsertionPoint []string
	Result         map[string]interface{}
	StripNode      bool
}

// execution is broken up into two phases:
// - the first walks down the dependency graph execute the network request
// - the second strips the id fields from the response and  provides a
//   place for certain middlewares to fire

// ExecutionContext is a well-type alternative to context.Context and provides the context
// for a particular execution.
type ExecutionContext struct {
	Plan               *QueryPlan
	Variables          map[string]interface{}
	RequestContext     context.Context
	RequestMiddlewares []graphql.NetworkMiddleware
}

// Execute returns the result of the query plan
func (executor *ParallelExecutor) Execute(ctx *ExecutionContext) (map[string]interface{}, error) {
	// a place to store the result
	result := map[string]interface{}{}

	// a channel to receive query results
	resultCh := make(chan *queryExecutionResult, 10)
	defer close(resultCh)

	// a wait group so we know when we're done with all of the steps
	stepWg := &sync.WaitGroup{}

	// and a channel for errors
	errMutex := &sync.Mutex{}
	errCh := make(chan error, 10)
	defer close(errCh)

	// a channel to close the goroutine
	closeCh := make(chan bool)
	defer close(closeCh)

	// a lock for reading and writing to the result
	resultLock := &sync.Mutex{}

	// if there are no steps after the root step, there is a problem
	if len(ctx.Plan.RootStep.Then) == 0 {
		return nil, errors.New("was given empty plan")
	}

	// the root step could have multiple steps that have to happen
	for _, step := range ctx.Plan.RootStep.Then {
		stepWg.Add(1)
		go executeStep(ctx, ctx.Plan, step, []string{}, resultLock, ctx.Variables, resultCh, errCh, stepWg)
	}

	// the list of errors we have encountered while executing the plan
	errs := graphql.ErrorList{}

	// start a goroutine to add results to the list
	go func() {
		for {
			select {
			// we have a new result
			case payload := <-resultCh:
				if payload == nil {
					continue
				}
				log.Debug("Inserting result into ", payload.InsertionPoint)
				log.Debug("Result: ", payload.Result)

				// we have to grab the value in the result and write it to the appropriate spot in the
				// acumulator.
				err := executorInsertObject(result, resultLock, payload.InsertionPoint, payload.Result)
				if err != nil {
					errCh <- err
					continue
				}

				log.Debug("Done. ", result)
				// one of the queries is done
				stepWg.Done()

			case err := <-errCh:
				if err != nil {
					errMutex.Lock()
					// if the error was a list
					if errList, ok := err.(graphql.ErrorList); ok {
						errs = append(errs, errList...)
					} else {
						errs = append(errs, err)
					}
					errMutex.Unlock()
					stepWg.Done()
				}
			// we're done
			case <-closeCh:
				return
			}
		}
	}()

	// when the wait group is finished
	stepWg.Wait()

	// if we encountered any errors
	errMutex.Lock()
	nErrs := len(errs)
	defer errMutex.Unlock()

	if nErrs > 0 {
		return result, errs
	}

	// we didn't encounter any errors
	return result, nil
}

// TODO: ugh... so... many... variables...
func executeStep(
	ctx *ExecutionContext,
	plan *QueryPlan,
	step *QueryPlanStep,
	insertionPoint []string,
	resultLock *sync.Mutex,
	queryVariables map[string]interface{},
	resultCh chan *queryExecutionResult,
	errCh chan error,
	stepWg *sync.WaitGroup,
) {
	log.Debug("")
	log.Debug("Executing step to be inserted in ", step.ParentType, ". Insertion point: ", insertionPoint)

	log.Debug(step.SelectionSet)

	// log the query
	log.QueryPlanStep(step)

	// the list of variables and their definitions that pertain to this query
	variables := map[string]interface{}{}

	// we need to grab the variable definitions and values for each variable in the step
	for variable := range step.Variables {
		// and the value if it exists
		if value, ok := queryVariables[variable]; ok {
			variables[variable] = value
		}
	}

	// the id of the object we are query is defined by the last step in the realized insertion point
	if len(insertionPoint) > 0 {
		head := insertionPoint[max(len(insertionPoint)-1, 0)]

		// get the data of the point
		pointData, err := executorGetPointData(head)
		if err != nil {
			errCh <- err
			return
		}

		// if we dont have an id
		if pointData.ID == "" {
			errCh <- fmt.Errorf("Could not find id in path")
			return
		}

		// save the id as a variable to the query
		variables["id"] = pointData.ID
	}

	// if there is no queryer
	if step.Queryer == nil {
		errCh <- errors.New(" could not find queryer for step")
		return
	}

	// the query we will use
	queryer := step.Queryer
	// a place to save the result
	queryResult := map[string]interface{}{}

	// if we have middlewares
	if len(ctx.RequestMiddlewares) > 0 {
		// if the queryer is a network queryer
		if nQueryer, ok := queryer.(graphql.QueryerWithMiddlewares); ok {
			queryer = nQueryer.WithMiddlewares(ctx.RequestMiddlewares)
		}
	}

	operationName := ""
	if plan != nil && plan.Operation != nil {
		operationName = plan.Operation.Name
	}

	// fire the query
	err := queryer.Query(ctx.RequestContext, &graphql.QueryInput{
		Query:         step.QueryString,
		QueryDocument: step.QueryDocument,
		Variables:     variables,
		OperationName: operationName,
	}, &queryResult)
	if err != nil {
		log.Warn("Network Error: ", err)
		errCh <- err
		return
	}

	// NOTE: this insertion point could point to a list of values. If it did, we have to have
	//       passed it to the this invocation of this function. It is safe to trust this
	//       InsertionPoint as the right place to insert this result.

	// if this is a query that falls underneath a `node(id: ???)` query then we only want to consider the object
	// underneath the `node` field as the result for the query
	stripNode := step.ParentType != "Query" && step.ParentType != "Subscription" && step.ParentType != "Mutation"
	if stripNode {
		log.Debug("Should strip node")
		// get the result from the response that we have to stitch there
		extractedResult, err := executorExtractValue(queryResult, resultLock, []string{"node"})
		if err != nil {
			errCh <- err
			return
		}

		resultObj, ok := extractedResult.(map[string]interface{})
		if !ok {
			errCh <- fmt.Errorf("Query result of node query was not an object: %v", queryResult)
			return
		}

		queryResult = resultObj
	}

	// if there are next steps
	if len(step.Then) > 0 {
		log.Debug("Kicking off child queries")
		// we need to find the ids of the objects we are inserting into and then kick of the worker with the right
		// insertion point. For lists, insertion points look like: ["user", "friends:0", "catPhotos:0", "owner"]
		for _, dependent := range step.Then {
			log.Debug("Looking for insertion points for ", dependent.InsertionPoint, "\n\n")

			insertPoints, err := executorFindInsertionPoints(resultLock, dependent.InsertionPoint, step.SelectionSet, queryResult, [][]string{insertionPoint}, step.FragmentDefinitions)
			if err != nil {
				errCh <- err
				return
			}

			// this dependent needs to fire for every object that the insertion point references
			for _, insertionPoint := range insertPoints {
				log.Info("Spawn ", insertionPoint)
				stepWg.Add(1)
				go executeStep(ctx, plan, dependent, insertionPoint, resultLock, queryVariables, resultCh, errCh, stepWg)
			}
		}
	}

	log.Debug("Pushing Result. Insertion point: ", insertionPoint, ". Value: ", queryResult)
	// send the result to be stitched in with our accumulator
	resultCh <- &queryExecutionResult{
		InsertionPoint: insertionPoint,
		Result:         queryResult,
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func findSelection(matchString string, selectionSet ast.SelectionSet, fragmentDefs ast.FragmentDefinitionList) (*ast.Field, error) {
	selectionSetFragments, err := graphql.ApplyFragments(selectionSet, fragmentDefs)
	if err != nil {
		return nil, err
	}

	for _, selection := range selectionSetFragments {
		switch selection := selection.(type) {
		case *ast.Field:
			if selection.Alias == matchString || selection.Name == matchString {
				return selection, nil
			}
		}
	}
	return nil, nil
}

// executorFindInsertionPoints returns the list of insertion points where this step should be executed.
func executorFindInsertionPoints(resultLock *sync.Mutex, targetPoints []string, selectionSet ast.SelectionSet, result map[string]interface{}, startingPoints [][]string, fragmentDefs ast.FragmentDefinitionList) ([][]string, error) {
	log.Debug("Looking for insertion points. target: ", targetPoints, " Starting from ", startingPoints)
	oldBranch := startingPoints

	// track the root of the selection set while  we walk
	selectionSetRoot := selectionSet

	// a place to refer to parts of the results
	resultChunk := result

	// the index to start at
	startingIndex := 0
	if len(oldBranch) > 0 {
		startingIndex = len(oldBranch[0])

		if len(targetPoints) == len(oldBranch[0]) {
			return startingPoints, nil
		}
	}

	log.Debug("First meaningful path point: ", targetPoints[startingIndex])
	log.Debug("result ", resultChunk)

	// if our starting point is []string{"users:0"} then we know everything so far
	// is along the path of the steps insertion point
	for pointI := startingIndex; pointI < len(targetPoints); pointI++ {
		// the point in the steps insertion path that we want to add
		point := targetPoints[pointI]

		log.Debug("Looking for ", point)

		// track wether we found a selection
		var foundSelection *ast.Field
		foundSelection, err := findSelection(point, selectionSetRoot, fragmentDefs)
		if err != nil {
			return [][]string{}, err
		}

		// if we didn't find a selection
		if foundSelection == nil {
			return [][]string{}, nil
		}

		log.Debug("")
		log.Debug("Found Selection for: ", point)
		log.Debug("Result Chunk: ", resultChunk)
		// make sure we are looking at the top of the selection set next time
		selectionSetRoot = foundSelection.SelectionSet

		var value = resultChunk

		// the bit of result chunk with the appropriate key should be a list
		rootValue, ok := value[point]
		if !ok {
			return [][]string{}, nil
		}

		// get the type of the object in question
		selectionType := foundSelection.Definition.Type

		// if the type is a list
		if selectionType.Elem != nil {
			log.Debug("Selection should be a list")
			// make sure the root value is a list
			rootList, ok := rootValue.([]interface{})
			if !ok {
				return nil, fmt.Errorf("Root value of result chunk was not a list: %v", rootValue)
			}

			// build up a new list of insertion points
			newInsertionPoints := [][]string{}

			// each value in the result contributes an insertion point
			for entryI, iEntry := range rootList {
				resultEntry, ok := iEntry.(map[string]interface{})
				if !ok {
					return nil, errors.New("entry in result wasn't a map")
				}

				// the point we are going to add to the list
				entryPoint := fmt.Sprintf("%s:%v", foundSelection.Name, entryI)
				log.Debug("Adding ", entryPoint, " to list")

				newBranchSet := make([][]string, len(oldBranch))
				copy(newBranchSet, oldBranch)

				// if we are adding to an existing branch
				if len(newBranchSet) > 0 {
					// add the path to the end of this for the entry we just added
					for i, newBranch := range newBranchSet {
						// if we are looking at the last thing in the insertion list
						if pointI == len(targetPoints)-1 {
							// look for an id
							id, ok := resultEntry["id"]
							if !ok {
								return nil, errors.New("Could not find the id for elements in target list")
							}

							// add the id to the entry so that the executor can use it to form its query
							entryPoint = fmt.Sprintf("%s#%v", entryPoint, id)

						}

						// add the point for this entry in the list
						newBranchSet[i] = append(newBranch, entryPoint)
					}
				} else {
					newBranchSet = append(newBranchSet, []string{entryPoint})
				}

				// compute the insertion points for that entry
				entryInsertionPoints, err := executorFindInsertionPoints(resultLock, targetPoints, selectionSetRoot, resultEntry, newBranchSet, fragmentDefs)
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
		// traverse down the resultChunk for the next iteration
		if rootValueMap, ok := rootValue.(map[string]interface{}); ok {
			resultChunk = rootValueMap
		}

		// we are encountering something that isn't a list so it must be an object or a scalar
		// regardless, we just need to add the point to the end of each list
		for i, points := range oldBranch {
			oldBranch[i] = append(points, point)
		}

		if pointI == len(targetPoints)-1 {
			// the root value could be a list in which case the id is the id of the corresponding entry
			// or the root value could be an object in which case the id is the id of the root value

			// if the root value is a list
			if rootList, ok := rootValue.([]interface{}); ok {
				for i := range oldBranch {
					entry, ok := rootList[i].(map[string]interface{})
					if !ok {
						return nil, errors.New("Item in root list isn't a map")
					}

					// look up the id of the object
					resultLock.Lock()
					id, ok := entry["id"]
					resultLock.Unlock()
					if !ok {
						return nil, errors.New("Could not find the id for the object")
					}

					// log.Debug("Adding id to ", oldBranch[i][pointI])

					oldBranch[i][pointI] = fmt.Sprintf("%s:%v#%v", oldBranch[i][pointI], i, id)

				}
			} else {
				rootObj, ok := rootValue.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("Root value of result chunk was not an object. Point: %v Value: %v", point, rootValue)
				}

				for i := range oldBranch {
					// look up the id of the object
					id := rootObj["id"]
					if !ok {
						return nil, errors.New("Could not find the id for the object")
					}

					oldBranch[i][pointI] = fmt.Sprintf("%s#%v", oldBranch[i][pointI], id)
				}
			}
		}

	}

	// return the aggregation
	return oldBranch, nil
}

func executorExtractValue(source map[string]interface{}, resultLock *sync.Mutex, path []string) (interface{}, error) {
	// a pointer to the objects we are modifying
	var recent interface{} = source
	log.Debug("Pulling ", path, " from ", source)

	for i, point := range path[:] {
		// if the point designates an element in the list
		if strings.Contains(point, ":") {
			pointData, err := executorGetPointData(point)
			if err != nil {
				return nil, err
			}

			recentObj, ok := recent.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("List was not a child of an object. %v", pointData)
			}

			// if the field does not exist
			if _, ok := recentObj[pointData.Field]; !ok {
				resultLock.Lock()
				recentObj[pointData.Field] = []interface{}{}
				resultLock.Unlock()
			}

			// it should be a list
			resultLock.Lock()
			field := recentObj[pointData.Field]
			resultLock.Unlock()

			targetList, ok := field.([]interface{})
			if !ok {
				return nil, fmt.Errorf("did not encounter a list when expected. Point: %v. Field: %v. Result %v", point, pointData.Field, field)
			}

			// if the field exists but does not have enough spots
			if len(targetList) <= pointData.Index {
				for i := len(targetList) - 1; i < pointData.Index; i++ {
					targetList = append(targetList, map[string]interface{}{})
				}

				// update the list with what we just made
				resultLock.Lock()
				recentObj[pointData.Field] = targetList
				resultLock.Unlock()
			}

			// focus on the right element
			resultLock.Lock()
			recent = targetList[pointData.Index]
			resultLock.Unlock()
		} else {
			// it's possible that there's an id
			pointData, err := executorGetPointData(point)
			if err != nil {
				return nil, err
			}

			pointField := pointData.Field

			recentObj, ok := recent.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("thisone, Target was not an object. %v, %v", pointData, recent)
			}

			// we are add an object value
			resultLock.Lock()
			targetObject := recentObj[pointField]
			resultLock.Unlock()

			if i != len(path)-1 && targetObject == nil {
				resultLock.Lock()
				recentObj[pointField] = map[string]interface{}{}
				resultLock.Unlock()
			}
			// if we haven't created an object there with that field
			if targetObject == nil {
				recentObj[pointField] = map[string]interface{}{}
			}

			// look there next
			recent = recentObj[pointField]
		}
	}

	return recent, nil
}

func executorInsertObject(target map[string]interface{}, resultLock *sync.Mutex, path []string, value interface{}) error {
	// log.Debug("Inserting object\n    Target: ", target, "\n    Path: ", path, "\n    Value: ", value)
	if len(path) > 0 {
		// a pointer to the objects we are modifying
		obj, err := executorExtractValue(target, resultLock, path)
		if err != nil {
			return err
		}

		targetObj, ok := obj.(map[string]interface{})
		if !ok {
			return errors.New("target object is not an object")
		}

		// if the value we are assigning is an object
		if newValue, ok := value.(map[string]interface{}); ok {
			for k, v := range newValue {
				resultLock.Lock()
				targetObj[k] = v
				resultLock.Unlock()
			}
		}
	} else {
		targetObj, ok := value.(map[string]interface{})
		if !ok {
			return errors.New("something went wrong")
		}

		for key, value := range targetObj {
			resultLock.Lock()
			target[key] = value
			resultLock.Unlock()
		}
	}
	return nil
}

type extractorPointData struct {
	Field string
	Index int
	ID    string
}

func executorGetPointData(point string) (*extractorPointData, error) {
	field := point
	index := -1
	id := ""

	// points come in the form <field>:<index>#<id> and each of index or id is optional
	if strings.Contains(point, "#") {
		idData := strings.Split(point, "#")
		if len(idData) == 2 {
			id = idData[1]
		}

		// use the index data without the id
		field = idData[0]
	}

	if strings.Contains(field, ":") {
		indexData := strings.Split(field, ":")
		indexValue, err := strconv.ParseInt(indexData[1], 0, 32)
		if err != nil {
			return nil, err
		}

		index = int(indexValue)
		field = indexData[0]
	}

	return &extractorPointData{
		Field: field,
		Index: index,
		ID:    id,
	}, nil
}

// ExecutorFunc wraps a function to be used as an executor.
type ExecutorFunc func(ctx *ExecutionContext) (map[string]interface{}, error)

// Execute invokes and returns the internal function
func (e ExecutorFunc) Execute(ctx *ExecutionContext) (map[string]interface{}, error) {
	return e(ctx)
}

// ErrExecutor always returnes the internal error.
type ErrExecutor struct {
	Error error
}

// Execute returns the internet error
func (e *ErrExecutor) Execute(ctx *ExecutionContext) (map[string]interface{}, error) {
	return nil, e.Error
}

// MockExecutor always returns a success with the provided value
type MockExecutor struct {
	Value map[string]interface{}
}

// Execute returns the provided value
func (e *MockExecutor) Execute(ctx *ExecutionContext) (map[string]interface{}, error) {
	return e.Value, nil
}
