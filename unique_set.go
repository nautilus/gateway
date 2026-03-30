package gateway

import (
	"maps"
	"slices"
)

type uniqueSet[Value comparable] map[Value]struct{}

func newSet[Value comparable](values []Value) uniqueSet[Value] {
	u := make(uniqueSet[Value])
	for _, value := range values {
		u[value] = struct{}{}
	}
	return u
}

func (u uniqueSet[Value]) Intersection(other uniqueSet[Value]) uniqueSet[Value] {
	intersection := make(uniqueSet[Value])
	for value := range u {
		if _, isSet := other[value]; isSet {
			intersection[value] = struct{}{}
		}
	}
	return intersection
}

func (u uniqueSet[Value]) Equal(other uniqueSet[Value]) bool {
	return maps.Equal(u, other)
}

func (u uniqueSet[Value]) ToSlice() []Value {
	return slices.Collect(maps.Keys(u))
}
