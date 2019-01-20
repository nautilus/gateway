package graphql

import (
	"errors"
	"strconv"
	"strings"

	"github.com/carted/graphql/language/printer"
	gAst "github.com/graphql-go/graphql/language/ast"
	"github.com/vektah/gqlparser/ast"
)

// PrintQuery creates a string representation of an operation
func PrintQuery(document *ast.QueryDocument) (string, error) {
	// grab the first operation in the document
	operation := document.Operations[0]

	// in order to print we are going to turn the vektah package Operation into the graphql-go ast node
	selectionSet, err := printerConvertSelectionSet(operation.SelectionSet)
	if err != nil {
		return "", err
	}

	// figure out the operation
	opName := gAst.OperationTypeQuery

	if operation.Operation == ast.Query {
		opName = gAst.OperationTypeQuery
	} else if operation.Operation == ast.Mutation {
		opName = gAst.OperationTypeMutation
	} else if operation.Operation == ast.Subscription {
		opName = gAst.OperationTypeSubscription
	}

	gOperation := &gAst.OperationDefinition{
		Kind:      "OperationDefinition",
		Operation: opName,
		Name: &gAst.Name{
			Kind:  "Name",
			Value: operation.Name,
		},
		SelectionSet: selectionSet,
	}

	gDocument := &gAst.Document{
		Kind:        "Document",
		Definitions: []gAst.Node{gOperation},
	}

	// if we have fragment definitions to add
	if len(document.Fragments) > 0 {
		for _, defn := range document.Fragments {
			selectionSet, err := printerConvertSelectionSet(defn.SelectionSet)
			if err != nil {
				return "", err
			}

			gDocument.Definitions = append(gDocument.Definitions, &gAst.FragmentDefinition{
				Kind: "FragmentDefinition",
				Name: &gAst.Name{
					Kind:  "Name",
					Value: defn.Name,
				},
				TypeCondition: &gAst.Named{
					Kind: "Named",
					Name: &gAst.Name{
						Kind:  "Name",
						Value: defn.TypeCondition,
					},
				},
				SelectionSet: selectionSet,
			})
		}
	}

	// if we have variables to define
	if len(operation.VariableDefinitions) > 0 {
		// build up a list of variable definitions
		gVarDefs := []*gAst.VariableDefinition{}

		for _, variable := range operation.VariableDefinitions {
			gVarDefs = append(gVarDefs, &gAst.VariableDefinition{
				Kind: "VariableDefinition",
				Variable: &gAst.Variable{
					Kind: "Variable",
					Name: &gAst.Name{
						Kind:  "Name",
						Value: variable.Variable,
					},
				},
				Type: &gAst.NonNull{
					Kind: "NonNull",
					Type: &gAst.Named{
						Kind: "Named",
						Name: &gAst.Name{
							Kind:  "Name",
							Value: "ID",
						},
					},
				},
			})
		}

		// set the
		gOperation.VariableDefinitions = gVarDefs
	}

	result, ok := printer.Print(gDocument).(string)
	if !ok {
		return "", errors.New("Did not return a string")
	}

	return strings.Replace(result, `"__NULL_VALUE__"`, "null", -1), nil
}

func printerConvertDirectiveList(dList ast.DirectiveList) ([]*gAst.Directive, error) {
	// the list of directives to apply
	directives := []*gAst.Directive{}

	for _, directive := range dList {
		// the list of arguments to add
		args := []*gAst.Argument{}

		for _, arg := range directive.Arguments {
			newArg, err := printerBuildValue(arg.Value)
			if err != nil {
				return nil, err
			}
			args = append(args, &gAst.Argument{
				Kind: "Argument",
				Name: &gAst.Name{
					Kind:  "Name",
					Value: arg.Name,
				},
				Value: newArg,
			})
		}

		directives = append(directives, &gAst.Directive{
			Kind: "Directive",
			Name: &gAst.Name{
				Kind:  "Name",
				Value: directive.Name,
			},
			Arguments: args,
		})
	}

	return directives, nil
}

func printerConvertField(selectedField *ast.Field) (*gAst.Field, error) {
	// the field to result
	field := &gAst.Field{
		Kind: "Field",
		Name: &gAst.Name{
			Kind:  "Name",
			Value: selectedField.Name,
		},
	}

	// if there is an alias
	if selectedField.Alias != selectedField.Name {
		field.Alias = &gAst.Name{
			Kind:  "Name",
			Value: selectedField.Alias,
		}
	}

	// if the selection has arguments
	if len(selectedField.Arguments) > 0 {
		// the list of arguments to add
		args := []*gAst.Argument{}

		for _, arg := range selectedField.Arguments {
			newArg, err := printerBuildValue(arg.Value)
			if err != nil {
				return nil, err
			}
			args = append(args, &gAst.Argument{
				Kind: "Argument",
				Name: &gAst.Name{
					Kind:  "Name",
					Value: arg.Name,
				},
				Value: newArg,
			})
		}

		field.Arguments = args
	}

	// if the selection has sub selections
	if len(selectedField.SelectionSet) > 0 {
		selectionSet, err := printerConvertSelectionSet(selectedField.SelectionSet)
		if err != nil {
			return nil, err
		}
		// add the selection set
		field.SelectionSet = selectionSet
	}

	// if there are directives applied
	if len(selectedField.Directives) > 0 {
		directives, err := printerConvertDirectiveList(selectedField.Directives)
		if err != nil {
			return nil, err
		}
		field.Directives = directives
	}

	return field, nil
}

func printerConvertInlineFragment(inlineFragment *ast.InlineFragment) (gAst.Selection, error) {
	selection, err := printerConvertSelectionSet(inlineFragment.SelectionSet)
	if err != nil {
		return nil, err
	}
	directives, err := printerConvertDirectiveList(inlineFragment.Directives)
	if err != nil {
		return nil, err
	}

	return &gAst.InlineFragment{
		Kind: "InlineFragment",
		TypeCondition: &gAst.Named{
			Kind: "Named",
			Name: &gAst.Name{
				Kind:  "Name",
				Value: inlineFragment.TypeCondition,
			},
		},
		SelectionSet: selection,
		Directives:   directives,
	}, nil
}

func printerConvertFragmentSpread(fragmentSpread *ast.FragmentSpread) (*gAst.FragmentSpread, error) {
	directives, err := printerConvertDirectiveList(fragmentSpread.Directives)
	if err != nil {
		return nil, err
	}

	return &gAst.FragmentSpread{
		Kind: "FragmentSpread",
		Name: &gAst.Name{
			Kind:  "Name",
			Value: fragmentSpread.Name,
		},
		Directives: directives,
	}, nil
}

// take a selection set from vektah to gAst
func printerConvertSelectionSet(selectionSet ast.SelectionSet) (*gAst.SelectionSet, error) {
	// the list of selections for this
	selections := []gAst.Selection{}

	for _, selection := range selectionSet {
		switch selection := selection.(type) {
		case *ast.Field:
			// convert the field
			field, err := printerConvertField(selection)
			if err != nil {
				return nil, err
			}

			// add the field to the selection
			selections = append(selections, field)
		case *ast.InlineFragment:
			fragment, err := printerConvertInlineFragment(selection)
			if err != nil {
				return nil, err
			}
			// add the field to the selection
			selections = append(selections, fragment)
		case *ast.FragmentSpread:
			fragment, err := printerConvertFragmentSpread(selection)
			if err != nil {
				return nil, err
			}
			// add the field to the selection
			selections = append(selections, fragment)
		}
	}

	return &gAst.SelectionSet{
		Kind:       "SelectionSet",
		Selections: selections,
	}, nil
}

func printerBuildValue(from *ast.Value) (gAst.Value, error) {
	if from.Kind == ast.Variable {
		return &gAst.Variable{
			Kind: "Variable",
			Name: &gAst.Name{
				Kind:  "Name",
				Value: from.Raw,
			},
		}, nil
	}

	if from.Kind == ast.IntValue {
		return &gAst.IntValue{
			Kind:  "IntValue",
			Value: from.Raw,
		}, nil
	}

	if from.Kind == ast.FloatValue {
		return &gAst.FloatValue{
			Kind:  "FloatValue",
			Value: from.Raw,
		}, nil
	}

	// ugh https://github.com/graphql-go/graphql/issues/178
	if from.Kind == ast.NullValue {
		return &gAst.EnumValue{
			Kind:  "EnumValue",
			Value: "null",
		}, nil
	}

	if from.Kind == ast.StringValue {
		return &gAst.StringValue{
			Kind:  "StringValue",
			Value: from.Raw,
		}, nil
	}

	if from.Kind == ast.BooleanValue {
		val, err := strconv.ParseBool(from.Raw)
		if err != nil {
			return nil, err
		}

		return &gAst.BooleanValue{
			Kind:  "BooleanValue",
			Value: val,
		}, nil
	}

	if from.Kind == ast.EnumValue {
		return &gAst.EnumValue{
			Kind:  "EnumValue",
			Value: from.Raw,
		}, nil
	}

	if from.Kind == ast.ListValue {
		// the list of values in the list
		values := []gAst.Value{}

		for _, child := range from.Children {
			val, err := printerBuildValue(child.Value)
			if err != nil {
				return nil, err
			}

			values = append(values, val)
		}

		return &gAst.ListValue{
			Kind:   "ListValue",
			Values: values,
		}, nil
	}

	if from.Kind == ast.ObjectValue {
		// the list of values in the list
		fields := []*gAst.ObjectField{}

		for _, child := range from.Children {
			val, err := printerBuildValue(child.Value)
			if err != nil {
				return nil, err
			}

			fields = append(fields, &gAst.ObjectField{
				Kind: "ObjectField",
				Name: &gAst.Name{
					Kind:  "Name",
					Value: child.Name,
				},
				Value: val,
			})
		}

		return &gAst.ObjectValue{
			Kind:   "ObjectValue",
			Fields: fields,
		}, nil
	}

	return nil, nil

}
