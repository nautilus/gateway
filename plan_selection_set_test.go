package gateway

import (
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func TestFindSmallestLocationIntersection(t *testing.T) {
	t.Parallel()
	const schemaStr = `
		type Query {
			foo: String
			node(id: ID!): Node
		}

		interface Node {
			id: ID!
		}

		type Bar implements Node {
			id: ID!
			bar: String  # field exclusive to A
		}

		type Baz implements Node {  # whole type exclusive to B
			id: ID!
			baz: String
			biff: Biff
		}

		type Biff implements Node {  # type exclusive to B and C
			id: ID!
			boo: String  # field exclusive to C
		}
	`
	locations := FieldURLMap{}
	const (
		locationA = "a"
		locationB = "b"
		locationC = "c"
	)
	locations.RegisterURL(typeNameQuery, "foo", locationA, locationB, locationC)
	locations.RegisterURL(typeNameQuery, "node", locationA, locationB, locationC)
	locations.RegisterURL("Node", "id", locationA, locationB, locationC)

	locations.RegisterURL("Bar", "bar", locationA)
	locations.RegisterURL("Bar", "id", locationA, locationB)
	locations.RegisterURL("Baz", "baz", locationB)
	locations.RegisterURL("Baz", "biff", locationB)
	locations.RegisterURL("Baz", "id", locationB)
	locations.RegisterURL("Biff", "boo", locationC)
	locations.RegisterURL("Biff", "id", locationB, locationC)

	for _, tc := range []struct {
		description     string
		query           string
		expectLocations []string
		expectErr       string
	}{
		{
			description: "one shared field",
			query: `query {
				foo
			}`,
			expectLocations: []string{locationA, locationB, locationC},
		},
		{
			description: "one nested shared field",
			query: `query($id: ID!) {
				node(id: $id) {
					... on Bar {
						id
					}
				}
			}`,
			expectLocations: []string{locationA, locationB},
		},
		{
			description: "one nested exclusive field",
			query: `query($id: ID!) {
				node(id: $id) {
					... on Bar {
						bar
					}
				}
			}`,
			expectLocations: []string{locationA},
		},
		{
			description: "one nested exclusive type",
			query: `query($id: ID!) {
				node(id: $id) {
					... on Baz {
						id
					}
				}
			}`,
			expectLocations: []string{locationB},
		},
		{
			description: "fragment spread",
			query: `query($id: ID!) {
				node(id: $id) {
					...MyFragment
				}
			}
			fragment MyFragment on Baz {
				id
			}
			`,
			expectLocations: []string{locationB},
		},
		{
			description: "stops iterating just before intersection is empty",
			query: `query($id: ID!) {
				node(id: $id) {
					... on Baz {
						biff {   # exclusive to B
							boo  # exclusive to C
						}
					}
				}
			}`,
			expectLocations: []string{locationB},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()
			schema := gqlparser.MustLoadSchema(&ast.Source{Input: schemaStr})
			query := gqlparser.MustLoadQuery(schema, tc.query)
			intersection, err := findSmallestLocationIntersection(query.Fragments, locations, query.Operations[0].SelectionSet)
			if tc.expectErr != "" {
				assert.EqualError(t, err, tc.expectErr)
				return
			}
			assert.NoError(t, err)
			intersectionSlice := intersection.ToSlice()
			sort.Strings(intersectionSlice)
			assert.Equal(t, tc.expectLocations, intersectionSlice)
		})
	}
}

func TestValidateUsedFragmentsAvailable(t *testing.T) {
	t.Parallel()
	const schemaStr = `
		type Query {
			foo: String
			bar: Bar
		}

		type Bar {
			baz: String
		}
	`
	for _, tc := range []struct {
		description     string
		query           string
		deleteFragments []string
		expectErr       string
	}{
		{
			description: "only fields",
			query: `query {
				foo
			}`,
		},
		{
			description: "only inline fragments",
			query: `query {
				... on Query {
					foo
				}
			}`,
		},
		{
			description: "only fragment spread",
			query: `
				query {
					...Biff
				}
				fragment Biff on Query {
					foo
				}
			`,
		},
		{
			description: "nested fields",
			query: `query {
				bar {
					baz
				}
			}`,
		},
		{
			description: "nested inline fragments",
			query: `query {
				... on Query {
					bar {
						... on Bar {
							baz
						}
					}
				}
			}`,
		},
		{
			description: "nested fragment spread",
			query: `
				query {
					...Biff
				}
				fragment Biff on Query {
					bar {
						...Boo
					}
				}
				fragment Boo on Bar {
					baz
				}
			`,
		},
		{
			description: "undefined fragment",
			query: `
				query {
					...Biff
				}
				fragment Biff on Query {
					foo
				}
			`,
			deleteFragments: []string{"Biff"},
			expectErr:       "fragment not found: Biff",
		},
		{
			description: "nested undefined fragment inside field",
			query: `
				query {
					bar {
						...Biff
					}
				}
				fragment Biff on Bar {
					baz
				}
			`,
			deleteFragments: []string{"Biff"},
			expectErr:       "fragment not found: Biff",
		},
		{
			description: "nested undefined fragment inside fragment spread",
			query: `
				query {
					...Biff
				}
				fragment Biff on Query {
					bar {
						...Boo
					}
				}
				fragment Boo on Bar {
					baz
				}
			`,
			deleteFragments: []string{"Boo"},
			expectErr:       "fragment not found: Boo",
		},
		{
			description: "nested undefined fragment inside inline fragment",
			query: `
				query {
					bar {
						... on Bar {
							...Biff
						}
					}
				}
				fragment Biff on Bar {
					baz
				}
			`,
			deleteFragments: []string{"Biff"},
			expectErr:       "fragment not found: Biff",
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()
			schema := gqlparser.MustLoadSchema(&ast.Source{Input: schemaStr})
			query := gqlparser.MustLoadQuery(schema, tc.query)
			op := query.Operations[0]
			fragments := slices.DeleteFunc(query.Fragments, func(fragment *ast.FragmentDefinition) bool {
				return slices.Contains(tc.deleteFragments, fragment.Name)
			})

			err := validateUsedFragmentsAvailable(fragments, op.SelectionSet)
			if tc.expectErr != "" {
				assert.EqualError(t, err, tc.expectErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}
