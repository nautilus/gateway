package gateway

import (
	"fmt"
	"strings"
	"sync"

	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"

	"github.com/alecaivazis/graphql-gateway/graphql"
)

// QueryPlanStep represents a step in the plan required to fulfill a query.
type QueryPlanStep struct {
	Queryer        graphql.Queryer
	ParentType     string
	ParentID       string
	SelectionSet   ast.SelectionSet
	InsertionPoint []string
	QueryDocument  *ast.OperationDefinition
	QueryString    string
	Then           []*QueryPlanStep
	Variables      Set
	// if this is set to true, we need to remove the id from the object that we are inserting the result into
	ClearID bool
}

// QueryPlan is the full plan to resolve a particular query
type QueryPlan struct {
	Operation *ast.OperationDefinition
	RootStep  *QueryPlanStep
}

type newQueryPlanStepPayload struct {
	ServiceName    string
	SelectionSet   ast.SelectionSet
	ParentType     string
	Parent         *QueryPlanStep
	InsertionPoint []string
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
			Operation: operation,
			RootStep:  &QueryPlanStep{},
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

		// the list of fields we care about
		fields := selectedFields(operation.SelectionSet)

		// get the type for the operation
		operationType := "Query"
		switch operation.Operation {
		case ast.Mutation:
			operationType = "Mutation"
		case ast.Subscription:
			operationType = "Subscription"
		}
		// start with one of the fields
		possibleLocations, err := locations.URLFor(operationType, fields[0].Name)
		if err != nil {
			return nil, err
		}

		currentLocation := possibleLocations[0]

		// we are garunteed at least one query
		stepWg.Add(1)

		// make sure that we apply any fragments before we start planning
		selectionSet, err := plannerApplyFragments(operation.SelectionSet, parsedQuery.Fragments)
		if err != nil {
			return nil, err
		}

		// start a new step
		stepCh <- &newQueryPlanStepPayload{
			SelectionSet:   selectionSet,
			ParentType:     operationType,
			ServiceName:    currentLocation,
			Parent:         plan.RootStep,
			InsertionPoint: []string{},
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
						Queryer:        p.GetQueryer(payload.ServiceName, schema),
						ParentType:     payload.ParentType,
						SelectionSet:   payload.SelectionSet,
						InsertionPoint: payload.InsertionPoint,
						Variables:      Set{},
					}

					// if there is a parent to this query
					if payload.Parent != nil {
						log.Debug(fmt.Sprintf("Adding step as dependency"))
						// add the new step to the Then of the parent
						payload.Parent.Then = append(payload.Parent.Then, step)
					}

					// log some stuffs
					selectionNames := []string{}
					for _, selection := range selectedFields(step.SelectionSet) {
						selectionNames = append(selectionNames, selection.Name)
					}

					log.Debug("")
					log.Debug(fmt.Sprintf("Encountered new step: %v with subquery (%v) @ %v \n", step.ParentType, strings.Join(selectionNames, ","), payload.InsertionPoint))

					// we are going to start walking down the operations selection set and let
					// the steps of the walk add any necessary selectedFields
					newSelection, err := p.extractSelection(&extractSelectionConfig{
						stepCh:         stepCh,
						stepWg:         stepWg,
						locations:      locations,
						parentLocation: payload.ServiceName,
						parentType:     step.ParentType,
						selection:      step.SelectionSet,
						step:           step,
						insertionPoint: payload.InsertionPoint,
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

					log.Debug("Step selection set:")
					for _, selection := range selectedFields(step.SelectionSet) {
						log.Debug(selection.Name)
					}
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
			for _, selection := range selectedFields(accumulator) {
				// if we still have to walk down the selection but we found the right branch
				if selection.Name == point {
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
		for _, selection := range selectedFields(accumulator) {
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

			// mark the id as artificially added
			step.ClearID = true
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
	step.QueryDocument = plannerBuildQuery(step.ParentType, variableDefs, step.SelectionSet)

	// we also need to turn the query into a string
	queryString, err := graphql.PrintQuery(step.QueryDocument)
	if err != nil {
		return err
	}
	step.QueryString = queryString

	// nothing went wrong here
	return nil
}

type extractSelectionConfig struct {
	stepCh chan *newQueryPlanStepPayload
	errCh  chan error
	stepWg *sync.WaitGroup

	locations      FieldURLMap
	parentLocation string
	parentType     string
	step           *QueryPlanStep
	selection      ast.SelectionSet
	insertionPoint []string
}

func (p *MinQueriesPlanner) extractSelection(config *extractSelectionConfig) (ast.SelectionSet, error) {
	// in order to group together fields in as few queries as possible, we need to group
	// the selection set by the location
	locationFields := map[string]ast.SelectionSet{}
	// we have to pass over this list twice so we can place selections that can go in more than one place
	fieldsLeft := []*ast.Field{}

	for _, field := range selectedFields(config.selection) {
		// look up the location for this field
		possibleLocations, err := config.locations.URLFor(config.parentType, field.Name)
		if err != nil {
			return nil, err
		}

		// if this field can only be found in one location
		if len(possibleLocations) == 1 {
			locationFields[possibleLocations[0]] = append(locationFields[possibleLocations[0]], field)
			// the field can be found in many locations
		} else {
			// add the field to fields for second pass
			fieldsLeft = append(fieldsLeft, field)
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
		// for now, just use the first one
		locationFields[possibleLocations[0]] = append(locationFields[possibleLocations[0]], field)
	}

	log.Debug("-----")
	log.Debug("Parent location: ", config.parentLocation)
	log.Debug("Locations: ", locationFields)

	// we have to make sure we spawn any more goroutines before this one terminates. This means that
	// we first have to look at any locations that are not the current one
	for location, fields := range locationFields {
		if location == config.parentLocation {
			continue
		}

		// we are dealing with a selection to another location that isn't the current one
		log.Debug(fmt.Sprintf("Adding the new step to resolve %s @ %v. Insertion point: %v\n", config.parentType, location, config.insertionPoint))

		// since we're adding another step we need to wait for at least one more goroutine to finish processing
		config.stepWg.Add(1)

		// add the new step
		config.stepCh <- &newQueryPlanStepPayload{
			ServiceName:    location,
			ParentType:     config.parentType,
			SelectionSet:   fields,
			Parent:         config.step,
			InsertionPoint: config.insertionPoint,
		}
	}

	// now we have to generate a selection set for fields that are coming from the same location as the parent
	currentLocationFields, ok := locationFields[config.parentLocation]
	if !ok {
		// there are no fields in the current location so we're done
		return nil, nil

	}

	// build up a selection set for the parent
	finalSelection := ast.SelectionSet{}

	// we need to repeat this process for each field in the current location selection set
	for _, field := range selectedFields(currentLocationFields) {
		// if the targetField has a selection, it cannot be added naively to the parent. We first have to
		// modify its selection set to only include fields that are at the same location as the parent.
		if len(field.SelectionSet) > 0 {
			// the insertion point for this field is the previous one with the new field name
			insertionPoint := make([]string, len(config.insertionPoint))
			copy(insertionPoint, config.insertionPoint)
			insertionPoint = append(insertionPoint, field.Alias)

			log.Debug("found a thing with a selection. extracting to ", insertionPoint, ". Parent insertion", config.insertionPoint)
			// add any possible selections provided by selections
			subSelection, err := p.extractSelection(&extractSelectionConfig{
				stepCh:         config.stepCh,
				stepWg:         config.stepWg,
				step:           config.step,
				locations:      config.locations,
				parentLocation: config.parentLocation,
				parentType:     coreFieldType(field).Name(),
				selection:      field.SelectionSet,
				insertionPoint: insertionPoint,
			})
			if err != nil {
				return nil, err
			}

			log.Debug(fmt.Sprintf("final selection for %s.%s: %v\n", config.parentType, field.Name, subSelection))

			// overwrite the selection set for this selection
			field.SelectionSet = subSelection
		} else {
			log.Debug("found a scalar")
		}
		// the field is now safe to add to the parents selection set

		// any variables that this field depends on need to be added to the steps list of variables
		for _, variable := range plannerExtractVariables(field.Arguments) {
			config.step.Variables.Add(variable)
		}

		// add it to the list
		finalSelection = append(finalSelection, field)
	}

	// we should have added every field that needs to be added to this list
	return finalSelection, nil
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

// Includes returns wether or not the string is in the set
func (set Set) Has(k string) bool {
	_, ok := set[k]

	return ok
}

// collectedFields are representations of a field with the list of selection sets that
// must be merged under that field.
type collectedField struct {
	*ast.Field
	NestedSelections []ast.SelectionSet
}

type collectedFieldList []*collectedField

func (c *collectedFieldList) GetOrCreateForAlias(alias string, creator func() *collectedField) *collectedField {
	// look for the field with the given alias
	for _, field := range *c {
		if field.Alias == alias {
			return field
		}
	}

	// if we didn't find a field with the chosen alias
	new := creator()

	// add the new field to the list
	*c = append(*c, new)

	return new
}

// applyFragments takes a list of selections and merges them into one, embedding any fragments it
// runs into along the way
func plannerApplyFragments(selectionSet ast.SelectionSet, fragmentDefs ast.FragmentDefinitionList) (ast.SelectionSet, error) {
	// build up a list of selection sets
	final := ast.SelectionSet{}

	// look for all of the collected fields
	collectedFields, err := plannerCollectFields([]ast.SelectionSet{selectionSet}, fragmentDefs)
	if err != nil {
		return nil, err
	}

	// the final result of collecting fields should have a single selection in its selection set
	// which should be a selection for the same field referenced by collected.Field
	for _, collected := range *collectedFields {
		final = append(final, collected.Field)
	}

	return final, nil
}

func plannerCollectFields(sources []ast.SelectionSet, fragments ast.FragmentDefinitionList) (*collectedFieldList, error) {
	// a way to look up field definitions and the list of selections we need under that field
	selectedFields := &collectedFieldList{}

	// each selection set we have to merge can contribute to selections for each field
	for _, selectionSet := range sources {
		for _, selection := range selectionSet {
			// a selection can be one of 3 things: a field, a fragment reference, or an inline fragment
			switch selection := selection.(type) {

			// a selection could either have a collected field or a real one. either way, we need to add the selection
			// set to the entry in the map
			case *ast.Field, *collectedField:
				var selectedField *ast.Field
				if field, ok := selection.(*ast.Field); ok {
					selectedField = field
				} else if collected, ok := selection.(*collectedField); ok {
					selectedField = collected.Field
				}

				// look up the entry in the field list for this field
				collected := selectedFields.GetOrCreateForAlias(selectedField.Alias, func() *collectedField {
					return &collectedField{Field: selectedField}
				})

				// add the fields selection set to the list
				collected.NestedSelections = append(collected.NestedSelections, selectedField.SelectionSet)

			// fragment selections need to be unwrapped and added to the final selection
			case *ast.InlineFragment, *ast.FragmentSpread:
				var selectionSet ast.SelectionSet

				// inline fragments
				if inlineFragment, ok := selection.(*ast.InlineFragment); ok {
					selectionSet = inlineFragment.SelectionSet

					// fragment spread
				} else if fragment, ok := selection.(*ast.FragmentSpread); ok {
					// grab the definition for the fragment
					definition := fragments.ForName(fragment.Name)
					if definition == nil {
						// this shouldn't happen since validation has already ran
						return nil, fmt.Errorf("Could not find fragment definition: %s", fragment.Name)
					}

					selectionSet = definition.SelectionSet
				}

				// fields underneath the inline fragment could be fragments themselves
				fields, err := plannerCollectFields([]ast.SelectionSet{selectionSet}, fragments)
				if err != nil {
					return nil, err
				}

				// each field in the inline fragment needs to be added to the selection
				for _, fragmentSelection := range *fields {
					// add the selection from the field to our accumulator
					collected := selectedFields.GetOrCreateForAlias(fragmentSelection.Alias, func() *collectedField {
						return fragmentSelection
					})

					// add the fragment selection set to the list of selections for the field
					collected.NestedSelections = append(collected.NestedSelections, fragmentSelection.SelectionSet)
				}
			}
		}
	}

	// each selected field needs to be merged into a single selection set
	for _, collected := range *selectedFields {
		// compute the new selection set for this field
		merged, err := plannerCollectFields(collected.NestedSelections, fragments)
		if err != nil {
			return nil, err
		}

		// if there are selections for the field we need to turn them into a selection set
		selectionSet := ast.SelectionSet{}
		for _, selection := range *merged {
			selectionSet = append(selectionSet, selection)
		}

		// save this selection set over the nested one
		collected.SelectionSet = selectionSet
	}

	// we're done
	return selectedFields, nil
}

func plannerExtractVariables(args ast.ArgumentList) []string {
	// the list of variables
	variables := []string{}

	// each argument could contain variables
	for _, arg := range args {
		plannerExtractVariablesFromValues(&variables, arg.Value)
	}

	// return the list
	return variables
}

func plannerExtractVariablesFromValues(accumulator *[]string, value *ast.Value) {
	// we have to look out for a few different kinds of values
	switch value.Kind {
	// if the value is a reference to a variable
	case ast.Variable:
		// add the ference to the list
		*accumulator = append(*accumulator, value.Raw)
	// the value could be a list
	case ast.ListValue, ast.ObjectValue:
		// each entry in the list or object could contribute a variable
		for _, child := range value.Children {
			plannerExtractVariablesFromValues(accumulator, child.Value)
		}
	}
}

func selectedFields(source ast.SelectionSet) []*ast.Field {
	// build up a list of fields
	fields := []*ast.Field{}

	// each source could contribute fields to this
	for _, selection := range source {
		// if we are selecting a field
		switch selection := selection.(type) {
		case *ast.Field:
			fields = append(fields, selection)
		case *collectedField:
			fields = append(fields, selection.Field)
		}
	}

	// we're done
	return fields
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

func plannerBuildQuery(parentType string, variables ast.VariableDefinitionList, selectionSet ast.SelectionSet) *ast.OperationDefinition {
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
	log.Debug("Build Query")

	// add the operation to a QueryDocument
	return operation
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
