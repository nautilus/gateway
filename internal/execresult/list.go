package execresult

import (
	"encoding/json"
	"iter"
	"sync/atomic"
)

type List struct {
	values atomic.Pointer[[]any]
}

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

func (l *List) Get(index int) (any, bool) {
	values := *l.values.Load()
	if index >= len(values) {
		return nil, false
	}
	return values[index], true
}

func (l *List) GetObjectAtIndex(index int) (*Object, bool) {
	value, largeEnough := l.Get(index)
	if !largeEnough {
		return nil, false
	}
	obj, ok := value.(*Object)
	return obj, ok
}

func (l *List) EnsureObjectAtIndex(index int) (*Object, bool) {
	l.ensureMinimumLength(index)
	return l.GetObjectAtIndex(index)
}

func (l *List) Length() int {
	return len(*l.values.Load())
}

func (l *List) All() iter.Seq2[int, any] {
	return func(yield func(int, any) bool) {
		for index, value := range *l.values.Load() {
			if !yield(index, value) {
				return
			}
		}
	}
}

func (l *List) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.values.Load())
}
