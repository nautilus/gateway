package gateway

import (
	"errors"
	"fmt"
	"sync"

	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"

	"github.com/nautilus/graphql"
)

// QueryPlanStep represents a step in the plan required to fulfill a query.
type QueryPlanStep struct {
	// execution meta data
	InsertionPoint []string
	Then           []*QueryPlanStep

	// required info to generate the query
	Queryer      graphql.Queryer
	ParentType   string
	ParentID     string
	SelectionSet ast.SelectionSet

	// pre-generated query stuff
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
	FieldsToScrub       map[string][][]string
}

type newQueryPlanStepPayload struct {
	Plan           *QueryPlan
	Location       string
	SelectionSet   ast.SelectionSet
	ParentType     string
	Parent         *QueryPlanStep
	InsertionPoint []string
	Fragments      ast.FragmentDefinitionList
	Wrapper        ast.SelectionSet
}

// QueryPlanner is responsible for taking a string with a graphql query and returns
// the steps to fulfill it
type QueryPlanner interface {
	Plan(*PlanningContext) (QueryPlanList, error)
}

// PlannerWithQueryerFactory is an interface for planners with configurable queryer factories
type PlannerWithQueryerFactory interface {
	WithQueryerFactory(*QueryerFactory) QueryPlanner
}

// QueryerFactory is a function that returns the queryer to use depending on the context
type QueryerFactory func(ctx *PlanningContext, url string) graphql.Queryer

// Planner is meant to be embedded in other QueryPlanners to share configuration
type Planner struct {
	QueryerFactory *QueryerFactory
	queryerCache   map[string]graphql.Queryer
}

// MinQueriesPlanner does the most basic level of query planning
type MinQueriesPlanner struct {
	Planner
}

// WithQueryerFactory returns a version of the planner with the factory set
func (p *MinQueriesPlanner) WithQueryerFactory(factory *QueryerFactory) QueryPlanner {
	p.Planner.QueryerFactory = factory
	return p
}

// PlanningContext is the input struct to the Plan method
type PlanningContext struct {
	Query     string
	Schema    *ast.Schema
	Locations FieldURLMap
	Gateway   *Gateway
}

// Plan computes the nested selections that will need to be performed
func (p *MinQueriesPlanner) Plan(ctx *PlanningContext) (QueryPlanList, error) {
	// the first thing to do is to parse the query
	parsedQuery, e := gqlparser.LoadQuery(ctx.Schema, ctx.Query)
	if e != nil {
		return nil, e
	}

	// generate the plan
	plans, err := p.generatePlans(ctx, parsedQuery)
	if err != nil {
		return nil, err
	}

	flatSelection, err := graphql.ApplyFragments(parsedQuery.Operations[0].SelectionSet, parsedQuery.Fragments)
	if err != nil {
		return nil, err
	}

	// add the scrub fields
	err = p.generateScrubFields(plans, flatSelection)
	if err != nil {
		return nil, err
	}

	// we're done
	return plans, nil
}

func (p *MinQueriesPlanner) generatePlans(ctx *PlanningContext, query *ast.QueryDocument) (QueryPlanList, error) {
	// an accumulator
	plans := QueryPlanList{}

	for _, operation := range query.Operations {
		// each operation results in a new query
		plan := &QueryPlan{
			Operation:           operation,
			FragmentDefinitions: query.Fragments,
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
			Wrapper:        ast.SelectionSet{},
		}

		// start waiting for steps to be added
		// NOTE: i dont think this closure is necessary ¯\_(ツ)_/¯
		go func(newSteps chan *newQueryPlanStepPayload) {
		SelectLoop:
			// continuously drain the step channel
			for {
				select {
				case payload, ok := <-newSteps:
					if !ok {
						return
					}
					step := &QueryPlanStep{
						Queryer:             p.GetQueryer(ctx, payload.Location),
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
						locations:      ctx.Locations,
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

					// build up the query document
					step.QueryDocument = plannerBuildQuery(plan.Operation.Name, step.ParentType, variableDefs, step.SelectionSet, step.FragmentDefinitions)

					// we also need to turn the query into a string
					queryString, err := graphql.PrintQuery(step.QueryDocument)
					if err != nil {
						errCh <- err
						continue SelectLoop
					}

					step.QueryString = queryString

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
		// there was an error
		case err := <-errCh:
			// bubble the error up
			return nil, err
		// we are done
		case <-doneCh:
			close(stepCh)
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
	wrapper        ast.SelectionSet
}

func (p *MinQueriesPlanner) extractSelection(config *extractSelectionConfig) (ast.SelectionSet, error) {
	log.Debug("")
	log.Debug("--- Extracting Selection ---")
	log.Debug("Parent location: ", config.parentLocation)

	// in order to group together fields in as few queries as possible, we need to group
	// the selection set by the location.
	locationFields, locationFragments, err := p.groupSelectionSet(config)
	if err != nil {
		return nil, err
	}

	log.Debug("Fields By Location: ", locationFields)

	// we only need to add an ID field if there are steps coming off of this insertion point
	checkForID := false

	// we have to make sure we spawn any more goroutines before this one terminates. This means that
	// we first have to look at any locations that are not the current one
	for location, selectionSet := range locationFields {
		if location == config.parentLocation {
			continue
		}

		// we are dealing with a selection to another location that isn't the current one
		log.Debug(fmt.Sprintf(
			"Adding the new step"+
				"\n\tParent Type: %s"+
				"\n\tLocation: %v"+
				"\n\tInsertion point: %v",
			config.parentType, location, config.insertionPoint))

		// if there are selections in this bundle that are not from the parent location we need to add
		// id to the selection set
		checkForID = true

		// if we have a wrapper to add
		if config.wrapper != nil && len(config.wrapper) > 0 {
			log.Debug("wrapping selection", config.wrapper)

			// use the wrapped version
			selectionSet, err = p.wrapSelectionSet(config, locationFragments, location, selectionSet)
			if err != nil {
				return nil, err
			}
		}

		// since we're adding another step we need to wait for at least one more goroutine to finish processing
		config.stepWg.Add(1)
		// add the new step
		config.stepCh <- &newQueryPlanStepPayload{
			Plan:           config.plan,
			Parent:         config.step,
			InsertionPoint: config.insertionPoint,
			Wrapper:        config.wrapper,
			ParentType:     config.parentType,

			Location:     location,
			SelectionSet: selectionSet,
			Fragments:    locationFragments[location],
		}
	}

	// if we have to have an id field on this selection set
	if checkForID {
		// add the id field since duplicates are ignored
		locationFields[config.parentLocation] = append(locationFields[config.parentLocation], &ast.Field{Name: "id"})
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

				// if this field is being wrapped in a fragment then we need to make sure
				// that any branches we kick off are still wrapped within the fragment.
				// if the field is being wrapped in any inline fragments (above or below),
				// we can get rid of them since the parent was responsible for handling
				wrapper := ast.SelectionSet{}
				if len(config.wrapper) > 0 {
					wrapper = config.wrapper[:1]
					if _, ok := wrapper[0].(*ast.InlineFragment); ok {
						wrapper = ast.SelectionSet{}
					}
				}

				log.Debug("found a thing with a selection. extracting to ", insertionPoint, ". Parent insertion", config.insertionPoint)
				// add any possible selections provided by this fields selections
				subSelection, err := p.extractSelection(&extractSelectionConfig{
					stepCh:         config.stepCh,
					stepWg:         config.stepWg,
					step:           config.step,
					locations:      config.locations,
					parentLocation: config.parentLocation,
					plan:           config.plan,

					parentType:     coreFieldType(selection).Name(),
					selection:      selection.SelectionSet,
					insertionPoint: insertionPoint,
					wrapper:        wrapper,
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

			// grab the official definition for the fragment.
			// we could have overwritten the definition to fit the local needs of the top level
			// ie if there is a branch off of one that happens mid-fragment.
			defn := config.step.FragmentDefinitions.ForName(selection.Name)
			addDefn := false
			if defn == nil {
				addDefn = true
				defn = config.plan.FragmentDefinitions.ForName(selection.Name)
			}

			// compute the actual selection set for the fragment coming from this location
			subSelection, err := p.extractSelection(&extractSelectionConfig{
				stepCh:         config.stepCh,
				stepWg:         config.stepWg,
				step:           config.step,
				locations:      config.locations,
				parentLocation: config.parentLocation,
				insertionPoint: config.insertionPoint,
				plan:           config.plan,

				parentType: defn.TypeCondition,
				selection:  defn.SelectionSet,
				// Children should now be wrapped by this fragment and nothing else
				wrapper: ast.SelectionSet{selection},
			})
			if err != nil {
				return nil, err
			}

			// if the step does not have a definition for this fragment
			if addDefn {
				// we're going to leave a different fragment definition behind for this step
				config.step.FragmentDefinitions = append(config.step.FragmentDefinitions,
					&ast.FragmentDefinition{
						Name:          selection.Name,
						TypeCondition: defn.TypeCondition,
						Directives:    defn.Directives,
					},
				)
			}

			// we need to make sure that this steps fragment definitions always match our expecatations
			config.step.FragmentDefinitions.ForName(selection.Name).SelectionSet = subSelection

		case *ast.InlineFragment:
			log.Debug("found an inline fragment. extracting to ", config.insertionPoint, ". Parent insertion", config.insertionPoint)

			newWrapper := make(ast.SelectionSet, len(config.wrapper))
			copy(newWrapper, config.wrapper)
			newWrapper = append(newWrapper, selection)

			// add any possible selections provided by selections
			subSelection, err := p.extractSelection(&extractSelectionConfig{
				stepCh:         config.stepCh,
				stepWg:         config.stepWg,
				step:           config.step,
				locations:      config.locations,
				parentLocation: config.parentLocation,
				plan:           config.plan,
				insertionPoint: config.insertionPoint,

				parentType: selection.TypeCondition,
				selection:  selection.SelectionSet,
				wrapper:    newWrapper,
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

func (p *MinQueriesPlanner) wrapSelectionSet(config *extractSelectionConfig, locationFragments map[string]ast.FragmentDefinitionList, location string, selectionSet ast.SelectionSet) (ast.SelectionSet, error) {

	log.Debug("wrapping selection", config.wrapper)

	// pointers required to nest the
	var selection ast.Selection
	var innerSelection ast.Selection

	for _, wrap := range config.wrapper {
		var newSelection ast.Selection

		switch wrap := wrap.(type) {
		case *ast.InlineFragment:
			// create a new inline fragment
			newSelection = &ast.InlineFragment{
				TypeCondition: wrap.TypeCondition,
				Directives:    wrap.Directives,
			}
		case *ast.FragmentSpread:
			newSelection = &ast.FragmentSpread{
				Name:       wrap.Name,
				Directives: wrap.Directives,
			}

			locationFragments[location] = append(locationFragments[location], &ast.FragmentDefinition{
				Name:          wrap.Name,
				TypeCondition: config.parentType,
			})
		}

		// if this is the first one then use the first object we create as the top level
		if selection == nil {
			selection = newSelection
		} else if sel, ok := innerSelection.(*ast.InlineFragment); ok {
			sel.SelectionSet = ast.SelectionSet{newSelection}
		} else if sel, ok := innerSelection.(*ast.FragmentSpread); ok {
			// look up the definition for the selection in the step
			defn := locationFragments[location].ForName(sel.Name)
			defn.SelectionSet = ast.SelectionSet{newSelection}
		}

		// this is the new inner-most selection
		innerSelection = newSelection
	}

	if sel, ok := innerSelection.(*ast.InlineFragment); ok {
		sel.SelectionSet = selectionSet
	} else if sel, ok := innerSelection.(*ast.FragmentSpread); ok {
		// look up the definition for the selection in the step
		defn := locationFragments[location].ForName(sel.Name)

		// if we couldn't find the definition
		if defn == nil {
			return nil, errors.New("Could not find defn")
		}

		// update its selection set
		defn.SelectionSet = selectionSet
	}

	return ast.SelectionSet{selection}, nil
}

// selects one location out of possibleLocations, prioritizing the parent's location and the internal schema
func (p *MinQueriesPlanner) selectLocation(possibleLocations []string, config *extractSelectionConfig) string {
	priority := map[string]bool{
		internalSchemaLocation: true,
		config.parentLocation:  true,
	}
	for _, location := range possibleLocations {
		if _, ok := priority[location]; ok {
			return location
		}
	}
	// if no priority is matched, the first location will be returned; this has a bias
	// towards the first location returned which results in uneven traffic for this location.
	return possibleLocations[0]
}

func (p *MinQueriesPlanner) groupSelectionSet(config *extractSelectionConfig) (map[string]ast.SelectionSet, map[string]ast.FragmentDefinitionList, error) {

	locationFields := map[string]ast.SelectionSet{}
	locationFragments := map[string]ast.FragmentDefinitionList{}

	// split each selection into groups of selection sets to be sent to a single service
	for _, selection := range config.selection {
		// each kind of selection contributes differently to the final selection set
		switch selection := selection.(type) {
		case *ast.Field:
			log.Debug("Encountered field ", selection.Name)

			field := &ast.Field{
				Name:             selection.Name,
				Alias:            selection.Alias,
				Directives:       selection.Directives,
				Arguments:        selection.Arguments,
				Definition:       selection.Definition,
				ObjectDefinition: selection.ObjectDefinition,
				SelectionSet:     selection.SelectionSet,
			}

			// look up the location for this field
			possibleLocations, err := config.locations.URLFor(config.parentType, selection.Name)
			if err != nil {
				return nil, nil, err
			}

			location := p.selectLocation(possibleLocations, config)
			locationFields[location] = append(locationFields[location], field)

		case *ast.FragmentSpread:
			log.Debug("Encountered fragment spread ", selection.Name)

			// a fragments fields can span multiple services so a single fragment can result in many selections being added
			fragmentLocations := map[string]ast.SelectionSet{}

			// look up if we already have a definition for this fragment in the step
			defn := config.step.FragmentDefinitions.ForName(selection.Name)

			// if we don't have it
			if defn == nil {
				// look in the operation
				defn = config.plan.FragmentDefinitions.ForName(selection.Name)
				if defn == nil {
					return nil, nil, fmt.Errorf("Could not find definition for directive: %s", selection.Name)
				}
			}

			// each field in the fragment should be bundled with whats around it (still wrapped in fragment)
			for _, fragmentSelection := range defn.SelectionSet {
				switch fragmentSelection := fragmentSelection.(type) {

				case *ast.Field:
					field := &ast.Field{
						Name:             fragmentSelection.Name,
						Alias:            fragmentSelection.Alias,
						Directives:       fragmentSelection.Directives,
						Arguments:        fragmentSelection.Arguments,
						Definition:       fragmentSelection.Definition,
						ObjectDefinition: fragmentSelection.ObjectDefinition,
						SelectionSet:     fragmentSelection.SelectionSet,
					}

					// look up the location of the field
					fieldLocations, err := config.locations.URLFor(defn.TypeCondition, field.Name)
					if err != nil {
						return nil, nil, err
					}

					fieldLocation := p.selectLocation(fieldLocations, config)
					fragmentLocations[fieldLocation] = append(fragmentLocations[fieldLocation], field)

				case *ast.FragmentSpread, *ast.InlineFragment:
					// non-field selections will be handled in the next tick
					// add it to the current location so we don't create a new step if its not needed
					fragmentLocations[config.parentLocation] = append(fragmentLocations[config.parentLocation], fragmentSelection)
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
						return nil, nil, err
					}

					// add the field to the location
					fragmentLocations[fieldLocations[0]] = append(fragmentLocations[fieldLocations[0]], fragmentSelection)

				case *ast.FragmentSpread, *ast.InlineFragment:
					// non-field selections will be handled in the next tick
					// add it to the current location so we don't create a new step if its not needed
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

	return locationFields, locationFragments, nil
}

// This plan results in a query that has fields that were not explicitly asked for.
// In order for the executor to know what to filter out of the final reply,
// we have to leave behind paths to objects that need to be scrubbed.
func (p *MinQueriesPlanner) generateScrubFields(plans QueryPlanList, requestSelection ast.SelectionSet) error {
	for _, plan := range plans {
		// the list of fields to scrub in this plan
		fieldsToScrub := map[string][][]string{"id": {}}

		// add all of the plans for the next step along with those from this step
		for _, nextStep := range plan.RootStep.Then {
			// compute the fields that our children have to add
			childScrubs, err := p.generateScrubFieldsWalk(nextStep, requestSelection)
			if err != nil {
				return err
			}

			for field, values := range childScrubs {
				fieldsToScrub[field] = append(fieldsToScrub[field], values...)
			}
		}

		plan.FieldsToScrub = fieldsToScrub
	}

	return nil
}

func (p *MinQueriesPlanner) generateScrubFieldsWalk(step *QueryPlanStep, selection ast.SelectionSet) (map[string][][]string, error) {
	// the acumulator of plans
	acc := map[string][][]string{}

	insertionPoint := step.InsertionPoint
	targetSelection := selection

	// we need to look if this steps insertion point artificially asked for the id
	for _, point := range insertionPoint {
		foundField := false

		// look over the points in the selection
		for _, field := range graphql.SelectedFields(targetSelection) {
			// if the field name is what we expected
			if field.Name == point {
				// our next selection set is the fields selection set
				targetSelection = field.SelectionSet

				// we found the field for this point
				foundField = true
				break
			}
		}

		if !foundField {
			return nil, fmt.Errorf("error adding scrub fields: could not find field for point %s", point)
		}
	}

	// look through the selection for a field named id
	naturalID := false
	for _, field := range graphql.SelectedFields(targetSelection) {
		// if the field is for id
		if field.Alias == "id" {
			naturalID = true
		}
	}

	// if the id was not natural and we were going to be inserted somewhere
	if !naturalID && len(insertionPoint) > 0 {
		// we have to add this insertion point to the list places to scrub
		acc["id"] = append(acc["id"], insertionPoint)
	}

	// add all of the plans for the next step along with those from this step
	for _, nextStep := range step.Then {
		// compute the fields that our children have to add
		childScrubs, err := p.generateScrubFieldsWalk(nextStep, selection)
		if err != nil {
			return nil, err
		}

		for id, values := range childScrubs {
			acc[id] = append(acc[id], values...)
		}
	}

	return acc, nil
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
func (p *Planner) GetQueryer(ctx *PlanningContext, url string) graphql.Queryer {
	// if we are looking to query the local schema
	if url == internalSchemaLocation {
		return ctx.Gateway
	}

	// if there is a queryer factory defined
	if p.QueryerFactory != nil {
		// use the factory
		return (*p.QueryerFactory)(ctx, url)
	}

	// return the queryer for the url
	return graphql.NewSingleRequestQueryer(url)
}

func plannerBuildQuery(operationName, parentType string, variables ast.VariableDefinitionList, selectionSet ast.SelectionSet, fragmentDefinitions ast.FragmentDefinitionList) *ast.QueryDocument {
	log.Debug("Building Query: \n"+"\tParentType: ", parentType, " ")
	// build up an operation for the query
	operation := &ast.OperationDefinition{
		VariableDefinitions: variables,
		Name:                operationName,
	}

	// assign the right operation
	switch parentType {
	case "Mutation":
		operation.Operation = ast.Mutation
	case "Subscription":
		operation.Operation = ast.Subscription
	default:
		operation.Operation = ast.Query
	}

	// if we are querying an operation all we need to do is add the selection set at the root
	if parentType == "Query" || parentType == "Mutation" || parentType == "Subscription" {
		operation.SelectionSet = selectionSet
	} else {
		// if we are not querying the top level then we have to embed the selection set
		// under the node query with the right id as the argument

		// we want the operation to have the equivalent of
		// {
		//	 	node(id: $id) {
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

		// if the original query didn't have an id arg we need to add one
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

func (p *MockErrPlanner) Plan(*PlanningContext) (QueryPlanList, error) {
	return nil, p.Err
}

// MockPlanner always returns the provided list of plans. Useful in testing.
type MockPlanner struct {
	Plans QueryPlanList
}

func (p *MockPlanner) Plan(*PlanningContext) (QueryPlanList, error) {
	return p.Plans, nil
}

// QueryPlanList is a list of plans which can be indexed by operation name
type QueryPlanList []*QueryPlan

// ForOperation returns the query plan meant to satisfy the given operation name
func (l QueryPlanList) ForOperation(name string) (*QueryPlan, error) {
	// look over every plan in the list for the operation with the matching name
	for _, plan := range l {
		if plan.Operation.Name == name {
			return plan, nil
		}
	}

	return nil, errors.New("could not find query for operation " + name)
}
