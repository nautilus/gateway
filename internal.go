package gateway

import (
	"context"

	"github.com/99designs/gqlgen/graphql/introspection"
	"github.com/mitchellh/mapstructure"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/nautilus/graphql"
)

// internalSchemaLocation is the location that functions should take to identify a remote schema
// that points to the gateway's internal schema.
const internalSchemaLocation = "🎉"

// Introspection schema field names
const (
	introspectArgs              = "args"
	introspectDeprecationReason = "deprecationReason"
	introspectDescription       = "description"
	introspectEnumValues        = "enumValues"
	introspectFields            = "fields"
	introspectInputFields       = "inputFields"
	introspectInterfaces        = "interfaces"
	introspectIsDeprecated      = "isDeprecated"
	introspectKind              = "kind"
	introspectName              = "name"
	introspectOfType            = "ofType"
	introspectPossibleTypes     = "possibleTypes"
	introspectType              = "type"
)

// QueryField is a hook to add gateway-level fields to a gateway. Limited to only being able to resolve
// an id of an already existing type in order to keep business logic out of the gateway.
type QueryField struct {
	Name      string
	Type      *ast.Type
	Arguments ast.ArgumentDefinitionList
	Resolver  func(context.Context, map[string]interface{}) (string, error)
}

// Query takes a query definition and writes the result to the receiver
func (g *Gateway) Query(ctx context.Context, input *graphql.QueryInput, receiver interface{}) error {
	// a place to store the result
	result := map[string]interface{}{}

	// wrap the schema in something capable of introspection
	introspectionSchema := introspection.WrapSchema(g.schema)

	if input.QueryDocument == nil && input.Query != "" {
		internalSchema, err := g.internalSchema()
		if err != nil {
			return err
		}
		var loadErr gqlerror.List
		input.QueryDocument, loadErr = gqlparser.LoadQuery(internalSchema, input.Query)
		if len(loadErr) > 0 {
			return loadErr
		}
	}

	// for local stuff we don't care about fragment directives
	querySelection, err := graphql.ApplyFragments(input.QueryDocument.Operations[0].SelectionSet, input.QueryDocument.Fragments)
	if err != nil {
		return err
	}

	for _, field := range graphql.SelectedFields(querySelection) {
		switch field.Name {
		case "__schema":
			result[field.Alias] = g.introspectSchema(introspectionSchema, field.SelectionSet)
		case "__type":
			// there is a name argument to look up the type
			name := field.Arguments.ForName("name").Value.Raw

			// look for the type with the designated name
			var introspectedType *introspection.Type
			for _, schemaType := range introspectionSchema.Types() {
				if *schemaType.Name() == name {
					schemaTypeCopy := schemaType // copy loop var
					introspectedType = &schemaTypeCopy
					break
				}
			}

			// if we couldn't find the type
			if introspectedType == nil {
				result[field.Alias] = nil
			} else {
				// we found the type so introspect it
				result[field.Alias] = g.introspectType(introspectedType, field.SelectionSet)
			}
		// to get this far and not be one of the above means that the field is a query field
		default:

			// look for the right field
			for _, qField := range g.queryFields {
				if field.Name == qField.Name {
					// consolidate the arguments in something that's easy to use
					args := map[string]interface{}{}
					for _, arg := range field.Arguments {
						// resolve the value of the argument
						value, err := arg.Value.Value(input.Variables)
						if err != nil {
							return err
						}

						// save it fo rlater
						args[arg.Name] = value
					}

					// find the id of the entity
					id, err := qField.Resolver(ctx, args)
					if err != nil {
						return &graphql.Error{
							Message: err.Error(),
							Path:    []interface{}{field.Alias},
						}
					}

					// assign the id to the response
					result[field.Alias] = map[string]interface{}{"id": id}
				}
			}
		}
	}

	// assign the result under the data key to the receiver
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "json",
		Result:  receiver,
	})
	if err != nil {
		return err
	}

	err = decoder.Decode(result)
	if err != nil {
		return err
	}

	return nil
}

func (g *Gateway) introspectSchema(schema *introspection.Schema, selectionSet ast.SelectionSet) map[string]interface{} {
	// a place to store the result
	result := map[string]interface{}{}

	for _, field := range graphql.SelectedFields(selectionSet) {
		switch field.Alias {
		case "types":
			result[field.Alias] = g.introspectTypeSlice(schema.Types(), field.SelectionSet)
		case "queryType":
			result[field.Alias] = g.introspectType(schema.QueryType(), field.SelectionSet)
		case "mutationType":
			result[field.Alias] = g.introspectType(schema.MutationType(), field.SelectionSet)
		case "subscriptionType":
			result[field.Alias] = g.introspectType(schema.SubscriptionType(), field.SelectionSet)
		case "directives":
			result[field.Alias] = g.introspectDirectiveSlice(schema.Directives(), field.SelectionSet)
		}
	}

	return result
}

func (g *Gateway) introspectType(schemaType *introspection.Type, selectionSet ast.SelectionSet) map[string]interface{} {
	if schemaType == nil {
		return nil
	}

	// a place to store the result
	result := map[string]interface{}{}

	for _, field := range graphql.SelectedFields(selectionSet) {
		// the default behavior is to ignore deprecated fields
		includeDeprecated := false
		if passedValue := field.Arguments.ForName("includeDeprecated"); passedValue != nil && passedValue.Value.Raw == "true" {
			includeDeprecated = true
		}

		switch field.Name {
		case introspectKind:
			result[field.Alias] = schemaType.Kind()
		case introspectName:
			result[field.Alias] = schemaType.Name()
		case introspectDescription:
			result[field.Alias] = schemaType.Description()
		case introspectFields:
			result[field.Alias] = g.introspectFieldSlice(schemaType.Fields(includeDeprecated), field.SelectionSet)
		case introspectInterfaces:
			result[field.Alias] = g.introspectTypeSlice(schemaType.Interfaces(), field.SelectionSet)
		case introspectPossibleTypes:
			result[field.Alias] = g.introspectTypeSlice(schemaType.PossibleTypes(), field.SelectionSet)
		case introspectEnumValues:
			result[field.Alias] = g.introspectEnumValueSlice(schemaType.EnumValues(includeDeprecated), field.SelectionSet)
		case introspectInputFields:
			result[field.Alias] = g.introspectInputValueSlice(schemaType.InputFields(), field.SelectionSet)
		case introspectOfType:
			result[field.Alias] = g.introspectType(schemaType.OfType(), field.SelectionSet)
		}
	}
	return result
}

func (g *Gateway) introspectField(fieldDef introspection.Field, selectionSet ast.SelectionSet) map[string]interface{} {
	// a place to store the result
	result := map[string]interface{}{}

	for _, field := range graphql.SelectedFields(selectionSet) {
		switch field.Name {
		case introspectName:
			result[field.Alias] = fieldDef.Name
		case introspectDescription:
			result[field.Alias] = fieldDef.Description()
		case introspectArgs:
			result[field.Alias] = g.introspectInputValueSlice(fieldDef.Args, field.SelectionSet)
		case introspectType:
			result[field.Alias] = g.introspectType(fieldDef.Type, field.SelectionSet)
		case introspectIsDeprecated:
			result[field.Alias] = fieldDef.IsDeprecated()
		case introspectDeprecationReason:
			result[field.Alias] = fieldDef.DeprecationReason()
		}
	}
	return result
}

func (g *Gateway) introspectEnumValue(definition *introspection.EnumValue, selectionSet ast.SelectionSet) map[string]interface{} {
	// a place to store the result
	result := map[string]interface{}{}

	for _, field := range graphql.SelectedFields(selectionSet) {
		switch field.Name {
		case introspectName:
			result[field.Alias] = definition.Name
		case introspectDescription:
			result[field.Alias] = definition.Description()
		case introspectIsDeprecated:
			result[field.Alias] = definition.IsDeprecated()
		case introspectDeprecationReason:
			result[field.Alias] = definition.DeprecationReason()
		}
	}

	return result
}

func (g *Gateway) introspectDirective(directive introspection.Directive, selectionSet ast.SelectionSet) map[string]interface{} {
	// a place to store the result
	result := map[string]interface{}{}

	for _, field := range graphql.SelectedFields(selectionSet) {
		switch field.Name {
		case introspectName:
			result[field.Alias] = directive.Name
		case introspectDescription:
			result[field.Alias] = directive.Description()
		case introspectArgs:
			result[field.Alias] = g.introspectInputValueSlice(directive.Args, field.SelectionSet)
		case "locations":
			result[field.Alias] = directive.Locations
		}
	}
	return result
}

func (g *Gateway) introspectInputValue(iv *introspection.InputValue, selectionSet ast.SelectionSet) map[string]interface{} {
	// a place to store the result
	result := map[string]interface{}{}

	for _, field := range graphql.SelectedFields(selectionSet) {
		switch field.Name {
		case introspectName:
			result[field.Alias] = iv.Name
		case introspectDescription:
			result[field.Alias] = iv.Description()
		case "type":
			result[field.Alias] = g.introspectType(iv.Type, field.SelectionSet)
		case "defaultValue":
			result[field.Alias] = iv.DefaultValue
		}
	}

	return result
}

func (g *Gateway) introspectInputValueSlice(values []introspection.InputValue, selectionSet ast.SelectionSet) []map[string]interface{} {
	result := []map[string]interface{}{}

	// each type in the schema
	for _, field := range values {
		field := field // use loop-local address
		result = append(result, g.introspectInputValue(&field, selectionSet))
	}

	return result
}

func (g *Gateway) introspectFieldSlice(fields []introspection.Field, selectionSet ast.SelectionSet) []map[string]interface{} {
	result := []map[string]interface{}{}

	// each type in the schema
	for _, field := range fields {
		result = append(result, g.introspectField(field, selectionSet))
	}

	return result
}

func (g *Gateway) introspectEnumValueSlice(values []introspection.EnumValue, selectionSet ast.SelectionSet) []map[string]interface{} {
	result := []map[string]interface{}{}

	// each type in the schema
	for _, enumValue := range values {
		enumValue := enumValue // use loop-local address
		result = append(result, g.introspectEnumValue(&enumValue, selectionSet))
	}

	return result
}

func (g *Gateway) introspectTypeSlice(types []introspection.Type, selectionSet ast.SelectionSet) []map[string]interface{} {
	result := []map[string]interface{}{}

	// each type in the schema
	for _, field := range types {
		field := field // use loop-local address
		result = append(result, g.introspectType(&field, selectionSet))
	}

	return result
}

func (g *Gateway) introspectDirectiveSlice(directives []introspection.Directive, selectionSet ast.SelectionSet) []map[string]interface{} {
	result := []map[string]interface{}{}

	// each type in the schema
	for _, directive := range directives {
		result = append(result, g.introspectDirective(directive, selectionSet))
	}

	return result
}
