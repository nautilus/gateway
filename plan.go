package gateway

import (
	"fmt"
	"sync"

	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"

	"github.com/alecaivazis/graphql-gateway/graphql"
)

// QueryPlanStep represents a step in the plan required to fulfill a query.
type QueryPlanStep struct {
	Queryer             graphql.Queryer
	ParentType          string
	ParentID            string
	SelectionSet        ast.SelectionSet
	InsertionPoint      []string
	Then                []*QueryPlanStep
	QueryDocument       *ast.QueryDocument
	QueryString         string
	FragmentDefinitions ast.FragmentDefinitionList
	Variables           Set
}

// QueryPlan is the full plan to resolve a particular query
type QueryPlan struct {
	Operation           *ast.OperationDefinition
	RootStep            *QueryPlanStep
	FragmentDefinitions ast.FragmentDefinitionList
}

type newQueryPlanStepPayload struct {
	Plan           *QueryPlan
	Location       string
	SelectionSet   ast.SelectionSet
	ParentType     string
	Parent         *QueryPlanStep
	InsertionPoint []string
	Fragments      ast.FragmentDefinitionList
	Wrapper        []*ast.InlineFragment
}

// QueryPlanner is responsible for taking a string with a graphql query and returns
// the steps to fulfill it
type QueryPlanner interface {
	Plan(string, *ast.Schema, FieldURLMap) ([]*QueryPlan, error)
}

// Planner is meant to be embedded in other QueryPlanners to share configuration
type Planner struct {
	QueryerFactory func(url string) graphql.Queryer
}

// MinQueriesPlanner does the most basic level of query planning
type MinQueriesPlanner struct {
	Planner
}

// Plan computes the nested selections that will need to be performed
func (p *MinQueriesPlanner) Plan(query string, schema *ast.Schema, locations FieldURLMap) ([]*QueryPlan, error) {
	// generating the plans happens in 2 steps.
	// - create the dependency graph of steps
	// - make sure that that each query has the required ids so that we can stitch results together

	// create the plans to satisfy the query
	plans, err := p.generatePlans(query, schema, locations)
	if err != nil {
		return nil, err
	}

	// we have to walk down each plan and make sure that the id field is found where it's needed
	for _, plan := range plans {
		p.preparePlanQueries(plan, plan.RootStep)
	}

	return plans, nil
}

func (p *MinQueriesPlanner) generatePlans(query string, schema *ast.Schema, locations FieldURLMap) ([]*QueryPlan, error) {
	// the first thing to do is to parse the query
	parsedQuery, err := gqlparser.LoadQuery(schema, query)
	if err != nil {
		return nil, err
	}

	// the list of plans that need to be executed simultaneously
	plans := []*QueryPlan{}

	for _, operation := range parsedQuery.Operations {
		// each operation results in a new query
		plan := &QueryPlan{
			Operation:           operation,
			FragmentDefinitions: parsedQuery.Fragments,
		}

		// add the plan to the top level list
		plans = append(plans, plan)

		// a channel to register new steps
		stepCh := make(chan *newQueryPlanStepPayload, 10)

		// a chan to get errors
		errCh := make(chan error)
		defer close(errCh)

		// a wait group to track the progress of goroutines
		stepWg := &sync.WaitGroup{}

		// get the type for the operation
		operationType := "Query"
		switch operation.Operation {
		case ast.Mutation:
			operationType = "Mutation"
		case ast.Subscription:
			operationType = "Subscription"
		}

		// we are garunteed at least one query
		stepWg.Add(1)

		// start with an empty root step
		stepCh <- &newQueryPlanStepPayload{
			Plan:           plan,
			SelectionSet:   operation.SelectionSet,
			ParentType:     operationType,
			Location:       "",
			InsertionPoint: []string{},
			Fragments:      ast.FragmentDefinitionList{},
			Wrapper:        []*ast.InlineFragment{},
		}

		// start waiting for steps to be added
		// NOTE: i dont think this closure is necessary ¯\_(ツ)_/¯
		go func(newSteps chan *newQueryPlanStepPayload) {
		SelectLoop:
			// continuously drain the step channel
			for {
				select {
				case payload := <-newSteps:
					step := &QueryPlanStep{
						Queryer:             p.GetQueryer(payload.Location, schema),
						ParentType:          payload.ParentType,
						SelectionSet:        ast.SelectionSet{},
						InsertionPoint:      payload.InsertionPoint,
						Variables:           Set{},
						FragmentDefinitions: payload.Fragments,
					}

					// if there is a parent to this query
					if payload.Parent != nil {
						log.Debug(fmt.Sprintf("Adding step as dependency"))
						// add the new step to the Then of the parent
						payload.Parent.Then = append(payload.Parent.Then, step)
					}
					// if we don't yet have a root step
					if plan.RootStep == nil {
						// use this one
						plan.RootStep = step
					}

					log.Debug(fmt.Sprintf(
						"Encountered new step: \n"+
							"\tParentType: %v \n"+
							"\tInsertion Point: %v \n"+
							"\tSelectionSet: \n%s",
						step.ParentType,
						payload.InsertionPoint,
						log.FormatSelectionSet(payload.SelectionSet),
					))

					// we are going to start walking down the operations selection set and let
					// the steps of the walk add any necessary selectedFields
					newSelection, err := p.extractSelection(&extractSelectionConfig{
						stepCh:         stepCh,
						stepWg:         stepWg,
						locations:      locations,
						parentLocation: payload.Location,
						parentType:     step.ParentType,
						selection:      payload.SelectionSet,
						step:           step,
						insertionPoint: payload.InsertionPoint,
						plan:           payload.Plan,
						wrapper:        payload.Wrapper,
					})
					if err != nil {
						errCh <- err
						continue SelectLoop
					}

					// if some of the fields are from the same location as the field on the operation
					if newSelection != nil {
						// we have a selection set from one of the root operation fields in the same location
						// so add it to the query we are sending to the service
						step.SelectionSet = newSelection
					}

					// now that we're done processing the step we need to preconstruct the query that we
					// will be firing for this plan

					// we need to grab the list of variable definitions
					variableDefs := ast.VariableDefinitionList{}
					// we need to grab the variable definitions and values for each variable in the step
					for variable := range step.Variables {
						// add the definition
						variableDefs = append(variableDefs, plan.Operation.VariableDefinitions.ForName(variable))
					}

					// we're done processing this step
					stepWg.Done()

					log.Debug("")
				}
			}
		}(stepCh)

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
		// we are done
		case <-doneCh:
			continue
		// there was an error
		case err := <-errCh:
			// bubble the error up
			return nil, err
		}
	}

	// return the final plan
	return plans, nil
}

type extractSelectionConfig struct {
	stepCh chan *newQueryPlanStepPayload
	errCh  chan error
	stepWg *sync.WaitGroup

	locations      FieldURLMap
	parentLocation string
	parentType     string
	step           *QueryPlanStep
	plan           *QueryPlan
	selection      ast.SelectionSet
	insertionPoint []string
	wrapper        []*ast.InlineFragment
}

func (p *MinQueriesPlanner) extractSelection(config *extractSelectionConfig) (ast.SelectionSet, error) {
	log.Debug("")
	log.Debug("--- Extracting Selection ---")
	log.Debug("Parent location: ", config.parentLocation)

	// in order to group together fields in as few queries as possible, we need to group
	// the selection set by the location.

	locationFields := map[string]ast.SelectionSet{}
	locationFragments := map[string]ast.FragmentDefinitionList{}

	// we have to pass over this list twice so we can place selections that can go in more than one place
	fieldsLeft := []*ast.Field{}

	// split each selection into groups of selection sets to be sent to a single service
	for _, selection := range config.selection {
		// each kind of selection contributes differently to the final selection set
		switch selection := selection.(type) {
		case *ast.Field:
			log.Debug("Encountered field ", selection.Name)

			// look up the location for this field
			possibleLocations, err := config.locations.URLFor(config.parentType, selection.Name)
			if err != nil {
				return nil, err
			}

			// if this field can only be found in one location
			if len(possibleLocations) == 1 {
				locationFields[possibleLocations[0]] = append(locationFields[possibleLocations[0]], selection)
				// the field can be found in many locations
			} else {
				// add the field to fields for second pass
				fieldsLeft = append(fieldsLeft, selection)
			}

		case *ast.FragmentSpread:
			log.Debug("Encountered fragment spread", selection.Name)

			// a fragments fields can span multiple services so a single fragment can result in many selections being added
			fragmentLocations := map[string]ast.SelectionSet{}

			// look up if we already have a definition for this fragment in the step
			defn := config.step.FragmentDefinitions.ForName(selection.Name)

			// if we don't have it
			if defn == nil {
				// look in the operation
				defn = config.plan.FragmentDefinitions.ForName(selection.Name)
				if defn == nil {
					return nil, fmt.Errorf("Could not find definition for directive: %s", selection.Name)
				}
			}

			// each field in the fragment should be bundled with whats around it (still wrapped in fragment)
			for _, fragmentSelection := range defn.SelectionSet {
				switch fragmentSelection := fragmentSelection.(type) {

				case *ast.Field:
					// look up the location of the field
					fieldLocations, err := config.locations.URLFor(defn.TypeCondition, fragmentSelection.Name)
					if err != nil {
						return nil, err
					}

					// add the field to the location
					fragmentLocations[fieldLocations[0]] = append(fragmentLocations[fieldLocations[0]], fragmentSelection)
				}
			}

			// for each bundle under a fragment
			for location, selectionSet := range fragmentLocations {
				// add the fragment spread to the selection set for this location
				locationFields[location] = append(locationFields[location], &ast.FragmentSpread{
					Name:       selection.Name,
					Directives: selection.Directives,
				})

				// since the fragment can only refer to fields in the top level that are at
				// the same location we need to add a new definition of the
				locationFragments[location] = append(locationFragments[location], &ast.FragmentDefinition{
					Name:          selection.Name,
					TypeCondition: defn.TypeCondition,
					SelectionSet:  selectionSet,
				})
			}

		case *ast.InlineFragment:
			log.Debug("Encountered inline fragment on ", selection.TypeCondition)

			// we need to split the inline fragment into an inline fragment for each location that this cover
			// and then add those inline fragments to the final selection

			fragmentLocations := map[string]ast.SelectionSet{}

			// each field in the fragment should be bundled with whats around it (still wrapped in fragment)
			for _, fragmentSelection := range selection.SelectionSet {
				switch fragmentSelection := fragmentSelection.(type) {
				case *ast.Field:
					// look up the location of the field
					fieldLocations, err := config.locations.URLFor(selection.TypeCondition, fragmentSelection.Name)
					if err != nil {
						return nil, err
					}

					// add the field to the location
					fragmentLocations[fieldLocations[0]] = append(fragmentLocations[fieldLocations[0]], fragmentSelection)

				case *ast.InlineFragment:
					// inline fragments within inline fragments will be dealt with next tick
					// add it to the current location selection so we don't create a new step
					fragmentLocations[config.parentLocation] = append(fragmentLocations[config.parentLocation], fragmentSelection)
				}
			}

			// for each bundle under a fragment
			for location, selectionSet := range fragmentLocations {
				// add the fragment spread to the selection set for this location
				locationFields[location] = append(locationFields[location], &ast.InlineFragment{
					TypeCondition: selection.TypeCondition,
					Directives:    selection.Directives,
					SelectionSet:  selectionSet,
				})
			}
		}
	}

	// each field that can go in more than one spot should be "favored" to go in the same location as the parent
FieldLoop:
	for _, field := range fieldsLeft {
		// look up the location for this field
		possibleLocations, err := config.locations.URLFor(config.parentType, field.Name)
		if err != nil {
			return nil, err
		}

		// look to see if the current location is one of the possible locations
		for _, location := range possibleLocations {
			// if the location is the same as the parent
			if location == config.parentLocation {
				// assign this field to the parents entry
				locationFields[location] = append(locationFields[location], field)
				// we're done with this field
				continue FieldLoop
			}
		}

		// if we got here then this field can be found in multiple services that are not the parent
		// just use the first one for now
		locationFields[possibleLocations[0]] = append(locationFields[possibleLocations[0]], field)
	}

	log.Debug("Fields By Location: ", locationFields)

	// we have to make sure we spawn any more goroutines before this one terminates. This means that
	// we first have to look at any locations that are not the current one
	for location, selectionSet := range locationFields {
		if location == config.parentLocation {
			continue
		}

		// if we have a wrapper to add
		if config.wrapper != nil && len(config.wrapper) > 0 {
			log.Debug("wrapping selection", config.wrapper)

			// pointers required to nest the
			var selection *ast.InlineFragment
			var innerSelection *ast.InlineFragment

			for _, wrap := range config.wrapper {
				// create a new inline fragment
				newSelection := &ast.InlineFragment{
					TypeCondition: wrap.TypeCondition,
					Directives:    wrap.Directives,
				}
				// if this is the first one then use the first object we create as the top level
				if selection == nil {
					selection = newSelection
				} else {
					innerSelection.SelectionSet = append(innerSelection.SelectionSet, newSelection)
				}

				// this is the new inner-most selection
				innerSelection = newSelection

				// if this is the first one then use the first object we create as the top level
				if selection == nil {
					selection = newSelection
				}
			}

			// add the original selection set
			innerSelection.SelectionSet = selectionSet

			// use the wrapped version
			selectionSet = ast.SelectionSet{selection}
		}

		// we are dealing with a selection to another location that isn't the current one
		log.Debug(fmt.Sprintf("Adding the new step to resolve %s @ %v. Insertion point: %v\n", config.parentType, location, config.insertionPoint))

		// since we're adding another step we need to wait for at least one more goroutine to finish processing
		config.stepWg.Add(1)

		// add the new step
		config.stepCh <- &newQueryPlanStepPayload{
			Plan:           config.plan,
			Location:       location,
			ParentType:     config.parentType,
			SelectionSet:   selectionSet,
			Fragments:      locationFragments[location],
			Parent:         config.step,
			InsertionPoint: config.insertionPoint,
			Wrapper:        config.wrapper,
		}
	}

	// now we have to generate a selection set for fields that are coming from the same location as the parent
	currentLocationFields, ok := locationFields[config.parentLocation]
	if !ok {
		// there are no fields in the current location so we're done
		return ast.SelectionSet{}, nil
	}

	// build up a selection set for the parent
	finalSelection := ast.SelectionSet{}

	// we need to repeat this process for each field in the current location selection set
	for _, selection := range currentLocationFields {
		switch selection := selection.(type) {
		case *ast.Field:
			// if the targetField has a selection, it cannot be added naively to the parent. We first have to
			// modify its selection set to only include fields that are at the same location as the parent.
			if len(selection.SelectionSet) > 0 {
				// the insertion point for this field is the previous one with the new field name
				insertionPoint := make([]string, len(config.insertionPoint))
				copy(insertionPoint, config.insertionPoint)
				insertionPoint = append(insertionPoint, selection.Alias)

				log.Debug("found a thing with a selection. extracting to ", insertionPoint, ". Parent insertion", config.insertionPoint)
				// add any possible selections provided by selections
				subSelection, err := p.extractSelection(&extractSelectionConfig{
					stepCh:    config.stepCh,
					stepWg:    config.stepWg,
					step:      config.step,
					locations: config.locations,

					parentLocation: config.parentLocation,
					parentType:     coreFieldType(selection).Name(),
					selection:      selection.SelectionSet,
					insertionPoint: insertionPoint,
					plan:           config.plan,
					// if this is a field, then its the one being wrapped. The children of this field
					// should not have the wrapper
					wrapper: nil,
				})
				if err != nil {
					return nil, err
				}

				log.Debug(fmt.Sprintf("final selection for %s.%s: %v\n", config.parentType, selection.Name, subSelection))

				// overwrite the selection set for this selection
				selection.SelectionSet = subSelection
			} else {
				log.Debug("found a scalar")
			}
			// the field is now safe to add to the parents selection set

			// any variables that this field depends on need to be added to the steps list of variables
			for _, variable := range graphql.ExtractVariables(selection.Arguments) {
				config.step.Variables.Add(variable)
			}

			// add it to the list
			finalSelection = append(finalSelection, selection)

		case *ast.FragmentSpread:
			// we have to walk down the fragments definition and keep adding to the selection sets and fragment definitions
			// add it to the list
			finalSelection = append(finalSelection, selection)

		case *ast.InlineFragment:
			// the insertion point for fields under this fragment is the same as this invocation
			insertionPoint := config.insertionPoint

			log.Debug("found an inline fragment. extracting to ", insertionPoint, ". Parent insertion", config.insertionPoint)

			newWrapper := make([]*ast.InlineFragment, len(config.wrapper))
			copy(newWrapper, config.wrapper)
			newWrapper = append(newWrapper, selection)

			// add any possible selections provided by selections
			subSelection, err := p.extractSelection(&extractSelectionConfig{
				stepCh:    config.stepCh,
				stepWg:    config.stepWg,
				step:      config.step,
				locations: config.locations,

				parentLocation: config.parentLocation,
				parentType:     selection.TypeCondition,
				selection:      selection.SelectionSet,
				insertionPoint: insertionPoint,
				plan:           config.plan,
				wrapper:        newWrapper,
			})

			if err != nil {
				return nil, err
			}

			// overwrite the selection set for this selection
			selection.SelectionSet = subSelection

			// for now, just add it to the list
			finalSelection = append(finalSelection, selection)
		}
	}
	// we should have added every field that needs to be added to this list
	return finalSelection, nil
}

func (p *MinQueriesPlanner) preparePlanQueries(plan *QueryPlan, step *QueryPlanStep) error {
	// we need to construct the query information for this step which requires adding ID's
	// where necessary to stitch results together

	// a step's query can be influenced by each step directly after it. In order for the
	// insertion point to work, there must be an id field in the object that
	for _, nextStep := range step.Then {

		// we need to walk down the graph
		for _, nextStep := range step.Then {
			err := p.preparePlanQueries(plan, nextStep)
			if err != nil {
				return err
			}
		}

		// if there is no selection
		if len(nextStep.InsertionPoint) == 0 {
			// ignore i
			continue
		}

		// the selection set we need to add `id` to
		accumulator := step.SelectionSet
		var targetField *ast.Field

		// walk down the list of insertion points
		for i := len(step.InsertionPoint); i < len(nextStep.InsertionPoint); i++ {
			// the point we are looking for in the selection set
			point := nextStep.InsertionPoint[i]

			// wether we found the corresponding field or not
			foundSelection := false

			// look for the selection with that name
			for _, selection := range graphql.SelectedFields(accumulator) {
				// if we still have to walk down the selection but we found the right branch
				if selection.Alias == point {
					accumulator = selection.SelectionSet
					targetField = selection
					foundSelection = true
					break
				}
			}

			if !foundSelection {
				return fmt.Errorf("Could not find selection for point: %s", point)
			}
		}

		// if we couldn't find the target
		if accumulator == nil {
			return fmt.Errorf("Could not find field to add id to. insertion point: %v", nextStep.InsertionPoint)
		}

		// if the target does not currently ask for id we need to add it
		addID := true
		for _, selection := range graphql.SelectedFields(accumulator) {
			if selection.Name == "id" {
				addID = false
				break
			}
		}

		// add the ID to the selection set if necessary
		if addID {
			accumulator = append(accumulator, &ast.Field{
				Name: "id",
			})
		}

		// make sure the selection set contains the id
		targetField.SelectionSet = accumulator
	}

	// we need to grab the list of variable definitions
	variableDefs := ast.VariableDefinitionList{}
	// we need to grab the variable definitions and values for each variable in the step
	for variable := range step.Variables {
		// add the definition
		variableDefs = append(variableDefs, plan.Operation.VariableDefinitions.ForName(variable))
	}

	// build up the query document
	step.QueryDocument = plannerBuildQuery(step.ParentType, variableDefs, step.SelectionSet, step.FragmentDefinitions)

	// we also need to turn the query into a string
	queryString, err := graphql.PrintQuery(step.QueryDocument)
	if err != nil {
		return err
	}
	step.QueryString = queryString

	// nothing went wrong here
	return nil
}

func coreFieldType(source *ast.Field) *ast.Type {
	// if we are looking at a
	return source.Definition.Type
}

// Set is a set
type Set map[string]bool

// Add adds the item to the set
func (set Set) Add(k string) {
	set[k] = true
}

// Remove removes the item from the set
func (set Set) Remove(k string) {
	delete(set, k)
}

// Has returns wether or not the string is in the set
func (set Set) Has(k string) bool {
	_, ok := set[k]

	return ok
}

// GetQueryer returns the queryer that should be used to resolve the plan
func (p *Planner) GetQueryer(url string, schema *ast.Schema) graphql.Queryer {
	// if we are looking to query the local schema
	if url == internalSchemaLocation {
		return &SchemaQueryer{Schema: schema}
	}

	// if there is a queryer factory defined
	if p.QueryerFactory != nil {
		// use the factory
		return p.QueryerFactory(url)
	}

	// otherwise return a network queryer
	return graphql.NewNetworkQueryer(url)
}

func plannerBuildQuery(parentType string, variables ast.VariableDefinitionList, selectionSet ast.SelectionSet, fragmentDefinitions ast.FragmentDefinitionList) *ast.QueryDocument {
	log.Debug("Querying ", parentType, " ")
	// build up an operation for the query
	operation := &ast.OperationDefinition{
		Operation:           ast.Query,
		VariableDefinitions: variables,
	}

	// if we are querying the top level Query all we need to do is add
	// the selection set at the root
	if parentType == "Query" {
		operation.SelectionSet = selectionSet
	} else {
		// if we are not querying the top level then we have to embed the selection set
		// under the node query with the right id as the argument

		// we want the operation to have the equivalent of
		// {
		//	 	node(id: parentID) {
		//	 		... on parentType {
		//	 			selection
		//	 		}
		//	 	}
		// }
		operation.SelectionSet = ast.SelectionSet{
			&ast.Field{
				Name: "node",
				Arguments: ast.ArgumentList{
					&ast.Argument{
						Name: "id",
						Value: &ast.Value{
							Kind: ast.Variable,
							Raw:  "id",
						},
					},
				},
				SelectionSet: ast.SelectionSet{
					&ast.InlineFragment{
						TypeCondition: parentType,
						SelectionSet:  selectionSet,
					},
				},
			},
		}

		if variables.ForName("id") == nil {
			operation.VariableDefinitions = append(operation.VariableDefinitions, &ast.VariableDefinition{
				Variable: "id",
				Type:     ast.NonNullNamedType("ID", &ast.Position{}),
			})
		}
	}

	// add the operation to a QueryDocument
	return &ast.QueryDocument{
		Operations: ast.OperationList{operation},
		Fragments:  fragmentDefinitions,
	}
}

// MockErrPlanner always returns the provided error. Useful in testing.
type MockErrPlanner struct {
	Err error
}

func (p *MockErrPlanner) Plan(string, *ast.Schema, FieldURLMap) ([]*QueryPlan, error) {
	return nil, p.Err
}

// MockPlanner always returns the provided list of plans. Useful in testing.
type MockPlanner struct {
	Plans []*QueryPlan
}

func (p *MockPlanner) Plan(string, *ast.Schema, FieldURLMap) ([]*QueryPlan, error) {
	return p.Plans, nil
}
