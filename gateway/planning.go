package gateway

import (
	"github.com/vektah/gqlparser"
	"github.com/vektah/gqlparser/ast"
)

// QueryPlanStep represents a step in the plan required to fulfill a query.
type QueryPlanStep struct {
	URL            string
	ParentType     string
	SelectionSet   ast.SelectionSet
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

		// the list of fields we care about
		fields := applyDirectives(operation.SelectionSet)

		// assume that the root location for this whole operation is the uniform
		possibleLocations, err := locations.URLFor("Query", fields[0].Name)
		if err != nil {
			return nil, err
		}

		currentLocation := possibleLocations[0]

		// add a single step to track down this root query
		plan.Steps = append(plan.Steps, &QueryPlanStep{
			URL:            currentLocation,
			SelectionSet:   ast.SelectionSet{},
			InsertLocation: "",
			DependsOn:      nil,
		})

		for _, selection := range applyDirectives(operation.SelectionSet) {
			// we are going to start walking down the operations selection set and let
			// the steps of the walk add any necessary selections
			selection, err := extractSelection(&extractSelectionConfig{
				stepAcc:        plan.Steps,
				locations:      locations,
				parentLocation: currentLocation,
				parentType:     "Query",
				field:          selection,
			})
			if err != nil {
				return nil, err
			}

			// if we got a selection back
			if selection != nil {
				plan.Steps[0].SelectionSet = append(plan.Steps[0].SelectionSet, selection)
			}

		}

	}

	// return the final plan
	return plans, nil
}

type extractSelectionConfig struct {
	stepAcc        []*QueryPlanStep
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

	// if the location of this targetField is the same as its parent
	if config.parentLocation == currentLocation {
		// if the targetField has subtargetFields and it cannot be added naively to the parent
		if len(config.field.SelectionSet) > 0 {

			// we are going to redefine this fields selection set
			newSelection := ast.SelectionSet{}

			// get the list of fields underneath the taret field
			for _, selection := range applyDirectives(config.field.SelectionSet) {
				// add any possible selections provided by selections
				subSelection, err := extractSelection(&extractSelectionConfig{
					stepAcc:        config.stepAcc,
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

			// overwrite the selection set for this selection
			config.field.SelectionSet = newSelection
		}

		// we should include this field regardless
		return config.field, nil
	}

	// we didn't encounter an error
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
