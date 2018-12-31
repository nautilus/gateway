package gateway

import (
	"sync"

	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"
)

// QueryPlanStep represents a step in the plan required to fulfill a query.
type QueryPlanStep struct {
	URL          string
	ParentType   string
	SelectionSet ast.SelectionSet
}

// QueryPlan is the full plan to resolve a particular query
type QueryPlan struct {
	Name  string
	Steps []*QueryPlanStep
}

// QueryPlanner is responsible for taking a parsed graphql string, and returning the steps to
// fulfill the response
type QueryPlanner interface {
	Plan(string, *ast.Schema, FieldURLMap) ([]*QueryPlan, error)
}

// NaiveQueryPlanner does the most basic level of query planning
type NaiveQueryPlanner struct{}

// Plan computes the nested selections that will need to be performed
func (p *NaiveQueryPlanner) Plan(query string, schema *ast.Schema, locations FieldURLMap) ([]*QueryPlan, error) {
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
			Name:  operation.Name,
			Steps: []*QueryPlanStep{},
		}

		// add the plan to the top level list
		plans = append(plans, plan)

		// the list of fields we care about
		fields := applyDirectives(operation.SelectionSet)

		// assume that the root location for this whole operation is the uniform
		possibleLocations, err := locations.URLFor("Query", fields[0].Name)
		if err != nil {
			return nil, err
		}

		currentLocation := possibleLocations[0]

		// a channel to register new steps
		stepCh := make(chan *QueryPlanStep, 10)

		// a chan to get errors
		errCh := make(chan error)

		// a wait group to track the progress of goroutines
		stepWg := &sync.WaitGroup{}

		// start waiting for steps to be added
		// NOTE: i dont think this closure is necessary ¯\_(ツ)_/¯
		go func(newSteps chan *QueryPlanStep) {
		SelectLoop:
			// contineously drain the step channel
			for {
				select {
				case step := <-newSteps:
					selectionNames := []string{}
					for _, selection := range applyDirectives(step.SelectionSet) {
						selectionNames = append(selectionNames, selection.Name)
					}
					// fmt.Printf("Encountered new step: %v with subquery [%v] @ %v \n", step.ParentType, strings.Join(selectionNames, ","), step.URL)
					// add it to the list of steps
					plan.Steps = append(plan.Steps, step)

					// the list of root selection steps
					selectionSet := ast.SelectionSet{}

					// for each field in the
					for _, selectedField := range applyDirectives(step.SelectionSet) {
						// we are going to start walking down the operations selectedField set and let
						// the steps of the walk add any necessary selectedFields
						// fmt.Println("extracting selection", selectedField.Name)
						newSelection, err := extractSelection(&extractSelectionConfig{
							stepCh:         stepCh,
							stepWg:         stepWg,
							locations:      locations,
							parentLocation: step.URL,
							parentType:     step.ParentType,
							field:          selectedField,
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

					// fmt.Println("Step selection set:")
					// for _, selection := range applyDirectives(step.SelectionSet) {
					// fmt.Println(selection.Name)
					// }
					// fmt.Println("     ")
					// we're done processing this step
					stepWg.Done()
				}
			}
		}(stepCh)

		// we are garunteed at least one query
		stepWg.Add(1)
		stepCh <- &QueryPlanStep{
			URL:          currentLocation,
			SelectionSet: operation.SelectionSet,
			ParentType:   "Query",
		}

		// there are 2 possible options:
		// - either the wait group finishes
		// - we get a messsage over the error chan

		// in order to wait for either, let's spawn a go routine
		// that waits for us, and notifies us when its done
		doneCh := make(chan bool)
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
	stepCh         chan *QueryPlanStep
	errCh          chan error
	stepWg         *sync.WaitGroup
	locations      FieldURLMap
	parentLocation string
	parentType     string
	field          *ast.Field
}

func extractSelection(config *extractSelectionConfig) (ast.Selection, error) {
	// look up the current location
	possibleLocations, err := config.locations.URLFor(config.parentType, config.field.Name)
	if err != nil {
		return nil, err
	}

	// grab the current one
	currentLocation := possibleLocations[0]

	// get the current type we are resolving
	currentType := coreFieldType(config.field).Name()

	// fmt.Println("-----")
	// fmt.Println("Looking at", config.field.Name)
	// if the location of this targetField is the same as its parent
	if config.parentLocation == currentLocation {
		// fmt.Println("same service")
		// if the targetField has subtargetFields and it cannot be added naively to the parent
		if len(config.field.SelectionSet) > 0 {
			// fmt.Println("found a thing with a selection")
			// we are going to redefine this fields selection set
			newSelection := ast.SelectionSet{}

			// get the list of fields underneath the taret field
			for _, selection := range applyDirectives(config.field.SelectionSet) {
				// add any possible selections provided by selections
				subSelection, err := extractSelection(&extractSelectionConfig{
					stepCh:         config.stepCh,
					stepWg:         config.stepWg,
					locations:      config.locations,
					parentLocation: currentLocation,
					parentType:     currentType,
					field:          selection,
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

			// fmt.Printf("final selection for %s.%s: %v\n", config.parentType, config.field.Name, newSelection)

			// overwrite the selection set for this selection
			config.field.SelectionSet = newSelection
		} else {
			// fmt.Println("found a scalar")
		}

		// we should include this field regardless
		return config.field, nil
	}

	// we're dealing with a field whose location does not match the parent

	// since we're adding another step we need to track at least one more execution
	config.stepWg.Add(1)
	// fmt.Printf("Adding the new step to resolve %s.%s\n", config.parentType, config.field.Name)

	// add the new step
	config.stepCh <- &QueryPlanStep{
		URL:          currentLocation,
		ParentType:   config.parentType,
		SelectionSet: ast.SelectionSet{config.field},
	}
	// we didn't encounter an error and shouldn't continue down this path
	return nil, nil
}

func coreFieldType(source *ast.Field) *ast.Type {
	// if we are looking at a
	return source.Definition.Type
}

func applyDirectives(source ast.SelectionSet) []*ast.Field {
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
