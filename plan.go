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
	Then           []*QueryPlanStep
	Variables      Set
}

// QueryPlan is the full plan to resolve a particular query
type QueryPlan struct {
	Operation string
	Variables ast.VariableDefinitionList
	RootStep  *QueryPlanStep
}

type newQueryPlanStepPayload struct {
	ServiceName    string
	SelectionSet   ast.SelectionSet
	ParentType     string
	Parent         *QueryPlanStep
	InsertionPoint []string
}

// QueryPlanner is responsible for taking a parsed graphql string, and returning the steps to
// fulfill the response
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
			Operation: operation.Name,
			Variables: operation.VariableDefinitions,
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

					// the list of root selection steps
					selectionSet := ast.SelectionSet{}

					// for each field in the
					for _, selectedField := range selectedFields(step.SelectionSet) {
						log.Debug("extracting selection ", selectedField.Name)
						// we always ignore the latest insertion point since we will add it to the list
						// in the extracts
						insertionPoint := []string{}
						if len(payload.InsertionPoint) != 0 {
							insertionPoint = payload.InsertionPoint[:len(payload.InsertionPoint)-1]
						}

						// we are going to start walking down the operations selection set and let
						// the steps of the walk add any necessary selectedFields
						newSelection, err := p.extractSelection(&extractSelectionConfig{
							stepCh:         stepCh,
							stepWg:         stepWg,
							locations:      locations,
							parentLocation: payload.ServiceName,
							parentType:     step.ParentType,
							field:          selectedField,
							step:           step,
							insertionPoint: insertionPoint,
						})
						if err != nil {
							errCh <- err
							continue SelectLoop
						}

						// if some of the fields are from the same location as the field on the operation
						if newSelection != nil {
							// we have a selection set from one of the root operation fields in the same location
							// so add it to the query we are sending to the service
							selectionSet = append(selectionSet, newSelection)
						}
					}

					// assign the new selection set
					step.SelectionSet = selectionSet

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

type extractSelectionConfig struct {
	stepCh chan *newQueryPlanStepPayload
	errCh  chan error
	stepWg *sync.WaitGroup

	locations      FieldURLMap
	parentLocation string
	parentType     string
	step           *QueryPlanStep
	field          *ast.Field
	insertionPoint []string
}

func (p *Planner) extractSelection(config *extractSelectionConfig) (ast.Selection, error) {
	// look up the current location
	possibleLocations, err := config.locations.URLFor(config.parentType, config.field.Name)
	if err != nil {
		return nil, err
	}

	// grab the current one
	currentLocation := possibleLocations[0]

	// get the current type we are resolving
	currentType := coreFieldType(config.field).Name()

	log.Debug("-----")
	log.Debug("Looking at: ", config.field.Name)
	log.Debug("Parent location: ", config.parentLocation)
	log.Debug("Current location: ", currentLocation)

	// the insertion point for this field is the previous one with the new field name
	insertionPoint := make([]string, len(config.insertionPoint))
	copy(insertionPoint, config.insertionPoint)
	insertionPoint = append(insertionPoint, config.field.Name)

	log.Debug(fmt.Sprintf("Insertion point: %v", insertionPoint))
	// if the location of this targetField is the same as its parent then we have to include it in the
	// selection set that we are building up.
	if config.parentLocation == currentLocation {
		log.Debug("same service")

		// if the targetField has a selection, it cannot be added naively to the parent. We first have to
		// modify its selection set to only include fields that are at the same location as the parent.
		if len(config.field.SelectionSet) > 0 {
			log.Debug("found a thing with a selection")
			// we are going to redefine this fields selection set
			newSelection := ast.SelectionSet{}

			// get the list of fields underneath the taret field
			for _, selection := range selectedFields(config.field.SelectionSet) {
				// add any possible selections provided by selections
				subSelection, err := p.extractSelection(&extractSelectionConfig{
					stepCh:         config.stepCh,
					stepWg:         config.stepWg,
					step:           config.step,
					locations:      config.locations,
					parentLocation: currentLocation,
					parentType:     currentType,
					field:          selection,
					insertionPoint: insertionPoint,
				})
				if err != nil {
					return nil, err
				}

				// if we got a selection
				if subSelection != nil {
					// add it to the list
					newSelection = append(newSelection, subSelection)
				}
			}

			log.Debug(fmt.Sprintf("final selection for %s.%s: %v\n", config.parentType, config.field.Name, newSelection))

			// overwrite the selection set for this selection
			config.field.SelectionSet = newSelection
		} else {
			log.Debug("found a scalar")
		}
		// the field is now safe to add to the parents selection set

		// any variables that this field depends on need to be added to the steps list of variables
		for _, variable := range plannerExtractVariables(config.field.Arguments) {
			config.step.Variables.Add(variable)
		}

		// return the field to be included in the parents selection
		return config.field, nil
	}

	// we're dealing with a field whose location does not match the parent

	// since we're adding another step we need to track at least one more execution
	config.stepWg.Add(1)
	log.Debug(fmt.Sprintf("Adding the new step to resolve %s.%s @%v\n", config.parentType, config.field.Name, currentLocation))

	// add the new step
	config.stepCh <- &newQueryPlanStepPayload{
		ServiceName:    currentLocation,
		ParentType:     config.parentType,
		SelectionSet:   ast.SelectionSet{config.field},
		Parent:         config.step,
		InsertionPoint: insertionPoint,
	}
	// we didn't encounter an error and dont have any fields to add to the parent
	return nil, nil
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
