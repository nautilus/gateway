package execresult

import (
	"encoding/json"
	"iter"
	"sync/atomic"
)

// List represents an execution result value. It's raw form is equivalent to []any
type List struct {
	values atomic.Pointer[[]any]
}

// NewList returns a new [List] with the given items converted into their execresult form of Objects and Lists
func NewList(items []any) *List {
	var objectTypeItems []any
	for _, item := range items {
		objectTypeItems = append(objectTypeItems, toObjectTypes(item))
	}
	var l List
	l.values.Store(&objectTypeItems)
	return &l
}

func (l *List) ensureMinimumLength(upToIndex int) {
	for {
		items := l.values.Load()
		if upToIndex < len(*items) {
			return
		}
		newItems := *items
		for range upToIndex + 1 - len(*items) {
			newItems = append(newItems, newWeakObject())
		}
		if l.values.CompareAndSwap(items, &newItems) {
			return
		}
	}
}

// Get returns l's value at the given index and true, or nil and false if index is out of bounds
func (l *List) Get(index int) (any, bool) {
	values := *l.values.Load()
	if index < 0 || index >= len(values) {
		return nil, false
	}
	return values[index], true
}

// GetObjectAtIndex returns l's object at the given index and true, or nil and false if the value is not an object or index is out of bounds.
func (l *List) GetObjectAtIndex(index int) (*Object, bool) {
	value, largeEnough := l.Get(index)
	if !largeEnough {
		return nil, false
	}
	obj, ok := value.(*Object)
	return obj, ok
}

// EnsureObjectAtIndex returns l's item at 'index' as an object or stores and returns a new weak object.
// Returns false if the value exists and is not an object or index is out of bounds.
func (l *List) EnsureObjectAtIndex(index int) (*Object, bool) {
	l.ensureMinimumLength(index)
	return l.GetObjectAtIndex(index)
}

// Length returns the number of items in l
func (l *List) Length() int {
	return len(*l.values.Load())
}

// All returns an iterator for all items in l
func (l *List) All() iter.Seq2[int, any] {
	return func(yield func(int, any) bool) {
		for index, value := range *l.values.Load() {
			if !yield(index, value) {
				return
			}
		}
	}
}

// MarshalJSON implements [json.Marshaler]
func (l *List) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.values.Load())
}
