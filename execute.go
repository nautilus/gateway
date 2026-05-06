package gateway

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/nautilus/gateway/internal/execresult"
	"github.com/nautilus/graphql"
	"github.com/vektah/gqlparser/v2/ast"
)

// Common type names for manipulating schemas
const (
	typeNameQuery        = "Query"
	typeNameMutation     = "Mutation"
	typeNameSubscription = "Subscription"
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
	Result         *execresult.Object
	Err            error
}

// StepHook is a function that can be executed before or after a step
type StepHook func(ctx *ExecutionContext) error

// execution is broken up into two phases:
// - the first walks down the dependency graph execute the network request
// - the second strips the id fields from the response and  provides a
//   place for certain middlewares to fire

// ExecutionContext is a well-type alternative to context.Context and provides the context
// for a particular execution.
type ExecutionContext struct {
	logger             Logger
	Plan               *QueryPlan
	Variables          map[string]interface{}
	RequestContext     context.Context
	RequestMiddlewares []graphql.NetworkMiddleware
	PreExecutionHook   StepHook
	PostExecutionHook  StepHook
}

// Execute returns the result of the query plan
func (executor *ParallelExecutor) Execute(ctx *ExecutionContext) (map[string]interface{}, error) {
	// a place to store the result
	result := execresult.NewObject()

	// a channel to receive query results
	const maxResultBuffer = 10
	resultCh := make(chan *queryExecutionResult, maxResultBuffer)
	defer close(resultCh)

	// a wait group so we know when we're done with all of the steps
	stepWg := &sync.WaitGroup{}

	// and a channel for errors
	errMutex := &sync.Mutex{}
	errCh := make(chan error, maxResultBuffer)
	defer close(errCh)

	// a channel to close the goroutine
	closeCh := make(chan bool)
	defer close(closeCh)

	// if there are no steps after the root step, there is a problem
	if len(ctx.Plan.RootStep.Then) == 0 {
		return nil, errors.New("was given empty plan")
	}

	// the root step could have multiple steps that have to happen
	for _, step := range ctx.Plan.RootStep.Then {
		stepWg.Add(1)
		go executeStep(ctx, ctx.Plan, step, []string{}, ctx.Variables, resultCh, stepWg)
	}

	// the list of errors we have encountered while executing the plan
	errs := graphql.ErrorList{}

	// start a goroutine to add results to the list
	go func() {
		for {
			select {
			// we have a new result
			case payload, ok := <-resultCh:
				if !ok {
					return
				}
				ctx.logger.Debug("Inserting result into ", payload.InsertionPoint)
				ctx.logger.Debug("Result: ", payload.Result)

				// we have to grab the value in the result and write it to the appropriate spot in the
				// acumulator.
				insertErr := executorInsertObject(ctx, result, payload.InsertionPoint, payload.Result)

				switch {
				case payload.Err != nil: // response errors are the highest priority to return
					errCh <- payload.Err
				case insertErr != nil:
					errCh <- insertErr
				default:
					ctx.logger.Debug("Done. ", result)
					// one of the queries is done
					stepWg.Done()
				}
			case err := <-errCh:
				if err != nil {
					errMutex.Lock()
					// if the error was a list
					var errList graphql.ErrorList
					if errors.As(err, &errList) {
						errs = append(errs, errList...)
					} else {
						ctx.logger.Warn("Unexpected error type executing query plan step: ", err)
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
		return result.ToMap(), errs
	}

	// we didn't encounter any errors
	return result.ToMap(), nil
}

// TODO: ugh... so... many... variables...
func executeStep(
	ctx *ExecutionContext,
	plan *QueryPlan,
	step *QueryPlanStep,
	insertionPoint []string,
	queryVariables map[string]interface{},
	resultCh chan *queryExecutionResult,
	stepWg *sync.WaitGroup,
) {
	queryResult, dependentSteps, queryErr := executeOneStep(ctx, plan, step, insertionPoint, queryVariables)
	// before publishing the current result, tell the wait-group about the dependent steps to wait for
	stepWg.Add(len(dependentSteps))
	ctx.logger.Debug("Pushing Result. Insertion point: ", insertionPoint, ". Value: ", queryResult)
	// send the result to be stitched in with our accumulator
	resultCh <- &queryExecutionResult{
		InsertionPoint: insertionPoint,
		Result:         queryResult,
		Err:            queryErr,
	}
	// We need to collect all the dependent steps and execute them after emitting the parent result in this function.
	// This avoids a race condition, where the result of a dependent request is published to the
	// result channel even before the result created in this iteration.
	// Execute dependent steps after the main step has been published.
	for _, sr := range dependentSteps {
		ctx.logger.Info("Spawn ", sr.insertionPoint)
		go executeStep(ctx, plan, sr.step, sr.insertionPoint, queryVariables, resultCh, stepWg)
	}
}

type dependentStepArgs struct {
	step           *QueryPlanStep
	insertionPoint []string
}

func executeOneStep(
	ctx *ExecutionContext,
	plan *QueryPlan,
	step *QueryPlanStep,
	insertionPoint []string,
	queryVariables map[string]interface{},
) (*execresult.Object, []dependentStepArgs, error) {
	ctx.logger.Debug("Executing step to be inserted in ", step.ParentType, ". Insertion point: ", insertionPoint)

	ctx.logger.Debug(step.SelectionSet)

	// log the query
	ctx.logger.QueryPlanStep(step)

	// Execute pre-execution hook if present
	if ctx.PreExecutionHook != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					ctx.logger.Warn("Pre-execution hook panicked: ", r)
				}
			}()
			if hookErr := ctx.PreExecutionHook(ctx); hookErr != nil {
				ctx.logger.Warn("Pre-execution hook failed: ", hookErr)
				// Continue execution even if pre-hook fails, but log the error
			}
		}()
	}

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
			return nil, nil, err
		}

		// if we dont have an id
		if pointData.ID == "" {
			return nil, nil, fmt.Errorf("could not find id in path")
		}

		// save the id as a variable to the query
		variables["id"] = pointData.ID
	}

	// if there is no queryer
	if step.Queryer == nil {
		return nil, nil, errors.New(" could not find queryer for step")
	}

	queryer := step.Queryer

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

	var queryResult *execresult.Object
	var queryErr error
	{ // fire the query
		var queryResultMap map[string]any
		queryErr = queryer.Query(ctx.RequestContext, &graphql.QueryInput{
			Query:         step.QueryString,
			QueryDocument: step.QueryDocument,
			Variables:     variables,
			OperationName: operationName,
		}, &queryResultMap)
		queryResult = execresult.NewObjectFromMap(queryResultMap)
	}

	// NOTE: this insertion point could point to a list of values. If it did, we have to have
	//       passed it to the this invocation of this function. It is safe to trust this
	//       InsertionPoint as the right place to insert this result.

	// if this is a query that falls underneath a `node(id: ???)` query then we only want to consider the object
	// underneath the `node` field as the result for the query
	stripNode := step.ParentType != typeNameQuery && step.ParentType != typeNameSubscription && step.ParentType != typeNameMutation
	if stripNode {
		ctx.logger.Debug("Should strip node")
		// get the result from the response that we have to stitch there
		extractedResult, err := executorExtractValue(ctx, queryResult, []string{"node"})
		if err != nil {
			return nil, nil, err
		}
		queryResult = extractedResult
	}

	// Execute post-execution hook if present
	if ctx.PostExecutionHook != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					ctx.logger.Warn("Post-execution hook panicked: ", r)
				}
			}()
			if hookErr := ctx.PostExecutionHook(ctx); hookErr != nil {
				ctx.logger.Warn("Post-execution hook failed: ", hookErr)
				// Continue execution even if post-hook fails, but log the error
			}
		}()
	}
	
	// if there are next steps
	var dependentSteps []dependentStepArgs
	if len(step.Then) > 0 {
		ctx.logger.Debug("Kicking off child queries")
		// we need to find the ids of the objects we are inserting into and then kick of the worker with the right
		// insertion point. For lists, insertion points look like: ["user", "friends:0", "catPhotos:0", "owner"]
		for _, dependent := range step.Then {
			copiedInsertionPoint := make([]string, len(insertionPoint))
			copy(copiedInsertionPoint, insertionPoint)
			insertPoints, missingIDPoints, err := executorFindInsertionPoints(ctx, dependent.InsertionPoint, step.SelectionSet, queryResult, [][]string{copiedInsertionPoint}, step.FragmentDefinitions)
			if err != nil {
				return nil, nil, err
			}
			if len(missingIDPoints) > 0 {
				return nil, nil, fmt.Errorf("could not find IDs for insertion points: %v", missingIDPoints)
			}

			// this dependent needs to fire for every object that the insertion point references
			for _, insertionPoint := range insertPoints {
				dependentSteps = append(dependentSteps, dependentStepArgs{
					step:           dependent,
					insertionPoint: insertionPoint,
				})
			}
		}
	}
	return queryResult, dependentSteps, queryErr
}

func findSelection(matchString string, selectionSet ast.SelectionSet, fragmentDefs ast.FragmentDefinitionList) (*ast.Field, error) {
	selectionSetFragments, err := graphql.ApplyFragments(selectionSet, fragmentDefs)
	if err != nil {
		return nil, err
	}

	for _, selection := range selectionSetFragments {
		selection, ok := selection.(*ast.Field)
		if ok && (selection.Alias == matchString || selection.Name == matchString) {
			return selection, nil
		}
	}

	return nil, nil
}

// executorFindInsertionPoints returns the list of insertion points where this step should be executed.
func executorFindInsertionPoints(ctx *ExecutionContext, targetPoints []string, selectionSet ast.SelectionSet, result *execresult.Object, startingPoints [][]string, fragmentDefs ast.FragmentDefinitionList) (insertionPoints [][]string, missingIDPoints [][]string, err error) {
	ctx.logger.Debug("Looking for insertion points. target: ", targetPoints, " Starting from ", startingPoints)
	startingIndex := 0
	if len(startingPoints) > 0 {
		startingIndex = len(startingPoints[0])

		if len(targetPoints) == len(startingPoints[0]) {
			return startingPoints, missingIDPoints, nil
		}
	}

	ctx.logger.Debug("traversing path point: ", targetPoints[startingIndex])

	// if our starting point is []string{"users:0"} then we know everything so far
	// is along the path of the steps insertion point
	point := targetPoints[startingIndex]
	isLastPoint := startingIndex == len(targetPoints)-1

	// find the selection node in the AST corresponding to the point
	foundSelection, err := findSelection(point, selectionSet, fragmentDefs)
	if err != nil {
		ctx.logger.Debug("Error looking for selection")
		return nil, nil, err
	}

	// if we didn't find a selection
	if foundSelection == nil {
		ctx.logger.Debug("No selection")
		return nil, missingIDPoints, nil
	}

	ctx.logger.Debug("Found Selection for: ", point)
	ctx.logger.Debug("Result Chunk: ", result)
	// make sure we are looking at the top of the selection set next time
	selectionSet = foundSelection.SelectionSet

	pointValue, ok := result.Get(point)
	if !ok {
		return nil, missingIDPoints, nil
	}

	// get the type of the object in question
	selectionType := foundSelection.Definition.Type

	if pointValue == nil {
		if selectionType.NonNull {
			err := fmt.Errorf("received null for required field: %v", foundSelection.Name)
			ctx.logger.Warn(err)
			return nil, nil, err
		}
		return nil, missingIDPoints, nil
	}

	if selectionType.Elem != nil {
		ctx.logger.Debug("Selection should be a list")
		list, ok := pointValue.(*execresult.List)
		if !ok {
			return nil, nil, fmt.Errorf("point value should be list, but was not: %v", pointValue)
		}

		// build up a new list of insertion points
		var newInsertionPoints [][]string

		// each value in the result contributes an insertion point
		for entryI, iEntry := range list.All() {
			resultEntry, ok := iEntry.(*execresult.Object)
			if !ok {
				return nil, nil, errors.New("entry in result wasn't an object")
			}

			// the point we are going to add to the list
			entryPoint := fmt.Sprintf("%s:%v", foundSelection.Name, entryI)
			if foundSelection.Alias != "" {
				entryPoint = fmt.Sprintf("%s:%v", foundSelection.Alias, entryI)
			}
			ctx.logger.Debug("Adding ", entryPoint, " to list")

			var newBranchSet [][]string
			for _, c := range startingPoints {
				newBranchSet = append(newBranchSet, copyStrings(c))
			}

			// if we are adding to an existing branch
			if len(newBranchSet) > 0 {
				notFoundIndices := make(map[int]struct{})
				// add the path to the end of this for the entry we just added
				for i, newBranch := range newBranchSet {
					branchEntryPoint := entryPoint // avoid mutating shared list entrypoint
					// if we are looking at the last thing in the insertion list
					if isLastPoint {
						// look for an id
						id, ok := resultEntry.Get("id")
						if !ok {
							notFoundIndices[i] = struct{}{}
						} else {
							// add the id to the entry so that the executor can use it to form its query
							branchEntryPoint = fmt.Sprintf("%s#%v", branchEntryPoint, id)
						}
					}
					newBranchSet[i] = append(newBranch, branchEntryPoint)
				}
				var deletedBranchSet [][]string
				newBranchSet, deletedBranchSet = deleteIndices(newBranchSet, notFoundIndices)
				missingIDPoints = append(missingIDPoints, deletedBranchSet...)
			} else {
				newBranchSet = append(newBranchSet, []string{entryPoint})
			}

			// compute the insertion points for that entry
			entryInsertionPoints, missingEntryIDPoints, err := executorFindInsertionPoints(ctx, targetPoints, selectionSet, resultEntry, newBranchSet, fragmentDefs)
			if err != nil {
				return nil, nil, err
			}

			// add the list of insertion points to the acumulator
			newInsertionPoints = append(newInsertionPoints, entryInsertionPoints...)
			missingIDPoints = append(missingIDPoints, missingEntryIDPoints...)
		}

		// return the flat list of insertion points created by our children
		return newInsertionPoints, missingIDPoints, nil
	}

	// traverse down the resultChunk for the next iteration
	if pointValueObj, ok := pointValue.(*execresult.Object); ok {
		result = pointValueObj
	}

	// we are encountering something that isn't a list so it must be an object or a scalar
	// regardless, we just need to add the point to the end of each list
	for i, points := range startingPoints {
		startingPoints[i] = append(points, point)
	}

	if isLastPoint {
		notFoundIndices := make(map[int]struct{})
		if list, ok := pointValue.(*execresult.List); ok {
			for i := range startingPoints {
				entry, ok := list.GetObjectAtIndex(i)
				if !ok {
					return nil, nil, errors.New("item in list isn't an object")
				}

				// look up the id of the object
				id, ok := entry.Get("id")
				if !ok {
					notFoundIndices[i] = struct{}{}
				}
				startingPoints[i][startingIndex] = fmt.Sprintf("%s:%v#%v", startingPoints[i][startingIndex], i, id)
			}
		} else {
			obj, ok := pointValue.(*execresult.Object)
			if !ok {
				return nil, nil, fmt.Errorf("point value was not an object. Point: %v Value: %v", point, pointValue)
			}
			for i := range startingPoints {
				// look up the id of the object
				id, ok := obj.Get("id")
				if !ok {
					notFoundIndices[i] = struct{}{}
				}
				startingPoints[i][startingIndex] = fmt.Sprintf("%s#%v", startingPoints[i][startingIndex], id)
			}
		}
		var deletedStartingPoints [][]string
		startingPoints, deletedStartingPoints = deleteIndices(startingPoints, notFoundIndices)
		missingIDPoints = append(missingIDPoints, deletedStartingPoints...)
	}
	insertionPoints, missingSubIDPoints, err := executorFindInsertionPoints(ctx, targetPoints, selectionSet, result, startingPoints, fragmentDefs)
	return insertionPoints, append(missingIDPoints, missingSubIDPoints...), err
}

func isListElement(path string) bool {
	if hashLocation := strings.Index(path, "#"); hashLocation > 0 {
		path = path[:hashLocation]
	}
	return strings.Contains(path, ":")
}

func executorExtractValue(ctx *ExecutionContext, source *execresult.Object, path []string) (*execresult.Object, error) {
	// a pointer to the objects we are modifying
	recent := source
	ctx.logger.Debug("Pulling ", path, " from ", source)

	for _, point := range path {
		// if the point designates an element in the list
		if isListElement(point) {
			pointData, err := executorGetPointData(point)
			if err != nil {
				return nil, err
			}

			list, ok := recent.EnsureList(pointData.Field)
			if !ok {
				value, _ := recent.Get(pointData.Field)
				return nil, fmt.Errorf("unexpected type at list insertion point %q: %T %v", pointData.Field, value, value)
			}
			obj, ok := list.EnsureObjectAtIndex(pointData.Index)
			if !ok {
				value, _ := list.Get(pointData.Index)
				return nil, fmt.Errorf("unexpected type at list item insertion point %q: %T %v", pointData.Field, value, value)
			}
			recent = obj
		} else {
			// it's possible that there's an id
			pointData, err := executorGetPointData(point)
			if err != nil {
				return nil, err
			}
			obj, ok := recent.EnsureObject(pointData.Field)
			if !ok {
				value, exists := recent.Get(pointData.Field)
				if exists && value == nil { // 'recent' is a strong object and field is already present and set to 'null'
					weakObj := execresult.NewObject()
					weakObj.SetWeak()
					return weakObj, nil
				}
				return nil, fmt.Errorf("target is non-null but not an object: %v, %T %v", pointData.Field, value, value)
			}
			recent = obj
		}
	}

	return recent, nil
}

func executorInsertObject(ctx *ExecutionContext, target *execresult.Object, path []string, value *execresult.Object) error {
	obj, err := executorExtractValue(ctx, target, path)
	if err != nil {
		return err
	}
	obj.MergeOverrides(value)
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
		const longIDParts = 2
		if len(idData) == longIDParts {
			id = idData[1]
		}

		// use the index data without the id
		field = idData[0]
	}

	if strings.Contains(field, ":") {
		indexData := strings.Split(field, ":")
		indexValue, err := strconv.Atoi(indexData[1])
		if err != nil {
			return nil, err
		}

		index = indexValue
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
func (e *ErrExecutor) Execute(_ *ExecutionContext) (map[string]interface{}, error) {
	return nil, e.Error
}

// MockExecutor always returns a success with the provided value
type MockExecutor struct {
	Value map[string]interface{}
}

// Execute returns the provided value
func (e *MockExecutor) Execute(_ *ExecutionContext) (map[string]interface{}, error) {
	return e.Value, nil
}

func copyStrings(s []string) []string {
	var result []string
	result = append(result, s...)
	return result
}

func deleteIndices[Value any](values []Value, indices map[int]struct{}) (newValues, deletedValues []Value) {
	for index, value := range values {
		if _, shouldDelete := indices[index]; shouldDelete {
			deletedValues = append(deletedValues, value)
		} else {
			newValues = append(newValues, value)
		}
	}
	return
}
