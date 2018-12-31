package gateway

import (
	"fmt"

	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"
)

// QueryPlanStep represents a step in the plan required to fulfill a query.
type QueryPlanStep struct {
	URL            string
	ParentType     string
	Field          *ast.Field
	InsertLocation string
	DependsOn      *QueryPlanStep
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

		// for now, count each top level field as an independent step
		for _, selection := range applyDirectives(operation.SelectionSet) {
			// look up the url for this field
			url, err := locations.URLFor("Query", selection.Name)
			if err != nil {
				return nil, err
			}

			plan.Steps = append(plan.Steps, &QueryPlanStep{
				URL:            url[0],
				Field:          selection,
				InsertLocation: "",
				DependsOn:      nil,
			})
		}

		// we are going to start walking down the selection set for each top level step in our plan
		// and let each step add more steps at the end if they want

		// visit each step to make sure more steps are needed
		for i := 0; i < len(plan.Steps); i++ {
			// the step in question
			step := plan.Steps[i]

			// grab a reference to the selection set
			selectionSet := step.Field.SelectionSet

			// clear the selection set for the field
			step.Field.SelectionSet = ast.SelectionSet{}

			// start walking down the selection set for the top level step and pass a reference to the
			// accumulator so each step can add new steps at the end
			err := walkSelectionSet(&planningWalkConfig{
				stepAcc:           plan.Steps,
				locations:         locations,
				parentLocation:    step.URL,
				parentType:        "Query",
				potentialChildren: selectionSet,
				targetField:       step.Field,
			})
			if err != nil {
				return nil, err
			}
		}

	}

	// return the final plan
	return plans, nil
}

type planningWalkConfig struct {
	stepAcc           []*QueryPlanStep
	locations         FieldURLMap
	parentLocation    string
	parentType        string
	potentialChildren ast.SelectionSet
	targetField       *ast.Field
}

func walkSelectionSet(config *planningWalkConfig) error {
	// look up the current location
	possibleLocations, err := config.locations.URLFor(config.parentType, config.targetField.Name)
	if err != nil {
		return err
	}

	// grab the current one
	currentLocation := possibleLocations[0]

	fmt.Println("looking at", config.parentType, config.targetField.Name)

	// get the current type we are resolving
	currentType := coreFieldType(config.targetField).Name()

	// if the location of this targetField is the same as its parent
	if config.parentLocation == currentLocation {
		// if the targetField has subtargetFields and it cannot be added naively to the parent
		if len(config.potentialChildren) > 0 {

			// get the list of fields underneath the taret field
			for _, selection := range applyDirectives(config.potentialChildren) {
				// the list of possible children for this selection
				potentialChildren := selection.SelectionSet

				// clear the selection set for the config.targetField
				selection.SelectionSet = ast.SelectionSet{}

				// add any possible selections provided by selections
				walkSelectionSet(&planningWalkConfig{
					stepAcc:           config.stepAcc,
					locations:         config.locations,
					parentLocation:    currentLocation,
					parentType:        currentType,
					potentialChildren: potentialChildren,
					targetField:       selection,
				})

				// add any selections to this object that our children designate
				config.targetField.SelectionSet = append(config.targetField.SelectionSet, selection)
			}
		}
	}

	// we didn't encounter an error
	return nil
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
