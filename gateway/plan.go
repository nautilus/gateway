package gateway

import (
	"fmt"
	"strings"
	"sync"

	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"
)

// QueryPlanStep represents a step in the plan required to fulfill a query.
type QueryPlanStep struct {
	Queryer        Queryer
	ParentType     string
	ParentID       string
	SelectionSet   ast.SelectionSet
	InsertionPoint []string
	Then           []*QueryPlanStep
}

// QueryPlan is the full plan to resolve a particular query
type QueryPlan struct {
	Operation string
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
	QueryerFactory func(url string) Queryer
}

// GetQueryer returns the queryer that should be used to resolve the plan
func (p *Planner) GetQueryer(url string) Queryer {
	// if there is a queryer factory defined
	if p.QueryerFactory != nil {
		// use the factory
		return p.QueryerFactory(url)
	}

	// otherwise return the network queryer
	return &NetworkQueryer{
		URL: url,
	}
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
			RootStep:  &QueryPlanStep{},
		}

		// add the plan to the top level list
		plans = append(plans, plan)

		// the list of fields we care about
		fields := applyFragments(operation.SelectionSet)

		// assume that the root location for this whole operation is the uniform
		possibleLocations, err := locations.URLFor("Query", fields[0].Name)
		if err != nil {
			return nil, err
		}

		currentLocation := possibleLocations[0]

		// a channel to register new steps
		stepCh := make(chan *newQueryPlanStepPayload, 10)

		// a chan to get errors
		errCh := make(chan error)
		defer close(errCh)

		// a wait group to track the progress of goroutines
		stepWg := &sync.WaitGroup{}

		// we are garunteed at least one query
		stepWg.Add(1)

		// start a new step
		stepCh <- &newQueryPlanStepPayload{
			SelectionSet:   operation.SelectionSet,
			ParentType:     "Query",
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
						Queryer:        p.GetQueryer(payload.ServiceName),
						ParentType:     payload.ParentType,
						SelectionSet:   payload.SelectionSet,
						InsertionPoint: payload.InsertionPoint,
					}

					// if there is a parent to this query
					if payload.Parent != nil {
						log.Debug(fmt.Sprintf("Adding step as dependency"))
						// add the new step to the Then of the parent
						payload.Parent.Then = append(payload.Parent.Then, step)
					}

					// log some stuffs
					selectionNames := []string{}
					for _, selection := range applyFragments(step.SelectionSet) {
						selectionNames = append(selectionNames, selection.Name)
					}

					log.Debug("")
					log.Debug(fmt.Sprintf("Encountered new step: %v with subquery (%v) @ %v \n", step.ParentType, strings.Join(selectionNames, ","), payload.InsertionPoint))

					// the list of root selection steps
					selectionSet := ast.SelectionSet{}

					// for each field in the
					for _, selectedField := range applyFragments(step.SelectionSet) {
						log.Debug("extracting selection ", selectedField.Name)
						// we always ignore the latest insertion point since we will add it to the list
						// in the extracts
						insertionPoint := []string{}
						if len(payload.InsertionPoint) != 0 {
							insertionPoint = payload.InsertionPoint[:len(payload.InsertionPoint)-1]
						}

						// we are going to start walking down the operations selectedField set and let
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
					for _, selection := range applyFragments(step.SelectionSet) {
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
	log.Debug("Looking at ", config.field.Name)

	// the insertion point for this field is the previous one with the new field name
	insertionPoint := make([]string, len(config.insertionPoint))
	copy(insertionPoint, config.insertionPoint)
	insertionPoint = append(insertionPoint, config.field.Name)

	log.Debug(fmt.Sprintf("Insertion point: %v", insertionPoint))

	// if the location of this targetField is the same as its parent
	if config.parentLocation == currentLocation {
		log.Debug("same service")
		// if the targetField has subtargetFields and it cannot be added naively to the parent
		if len(config.field.SelectionSet) > 0 {
			log.Debug("found a thing with a selection")
			// we are going to redefine this fields selection set
			newSelection := ast.SelectionSet{}

			// get the list of fields underneath the taret field
			for _, selection := range applyFragments(config.field.SelectionSet) {
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

		// we should include this field regardless
		return config.field, nil
	}

	// we're dealing with a field whose location does not match the parent

	// since we're adding another step we need to track at least one more execution
	config.stepWg.Add(1)
	log.Debug(fmt.Sprintf("Adding the new step to resolve %s.%s\n", config.parentType, config.field.Name))

	// add the new step
	config.stepCh <- &newQueryPlanStepPayload{
		ServiceName:    currentLocation,
		ParentType:     config.parentType,
		SelectionSet:   ast.SelectionSet{config.field},
		Parent:         config.step,
		InsertionPoint: insertionPoint,
	}
	// we didn't encounter an error and dont have any fields to add
	return nil, nil
}

func coreFieldType(source *ast.Field) *ast.Type {
	// if we are looking at a
	return source.Definition.Type
}

func applyFragments(source ast.SelectionSet) []*ast.Field {
	// build up a list of fields
	fields := []*ast.Field{}

	// each source could contribute fields to this
	for _, selection := range source {
		// if we are selecting a field
		switch selection := selection.(type) {
		case *ast.Field:
			fields = append(fields, selection)
		}
	}

	// we're done
	return fields
}
