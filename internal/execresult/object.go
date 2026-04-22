// Package execresult defines an execution result GraphQL object and list.
//
// All types may be used concurrently.
package execresult

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

type Object struct {
	fields sync.Map
	isWeak atomic.Bool
}

func NewObject() *Object {
	return &Object{}
}

// NewObjectFromMap returns an [Object] to represent the given map m and true. If m is nil, returns nil and false.
func NewObjectFromMap(m map[string]any) (*Object, bool) {
	obj, isNonNil := toObjectTypes(m).(*Object)
	return obj, isNonNil
}

// MustNewObjectFromMap is the same as [NewObjectFromMap] but panics if m is nil.
// Only safe for use in test cases.
func MustNewObjectFromMap(m map[string]any) *Object {
	return toObjectTypes(m).(*Object)
}

func toObjectTypes(v any) any {
	switch v := v.(type) {
	case map[string]any:
		if v == nil {
			return newWeakObject()
		}
		obj := NewObject()
		for key, value := range v {
			obj.Set(key, value)
		}
		return obj
	case []any:
		if v == nil {
			return nil
		}
		return NewList(v)
	case *Object, *List: // only encountered when crossing library API boundaries, like the Gateway Queryer implementation and the MinQueriesPlanner
		return v
	default:
		return v
	}
}

func newWeakObject() *Object {
	o := NewObject()
	o.SetWeak()
	return o
}

func (o *Object) SetWeak() {
	o.isWeak.Store(true)
}

func (o *Object) MergeOverrides(overrides *Object) {
	if o.isWeak.CompareAndSwap(true, false) {
		o.fields.Clear()
		if overrides == nil { // This object should become 'null' when marshaling to a map
			o.isWeak.Store(true)
		}
	}
	if overrides == nil {
		return
	}
	overrides.fields.Range(func(key, value any) bool {
		o.Set(key.(string), value)
		return true
	})
}

func (o *Object) ToMap() map[string]any {
	return toMap(o).(map[string]any)
}

// String implements [fmt.Stringer] for easy debugging
func (o *Object) String() string {
	return fmt.Sprint(o.ToMap())
}

func toMap(value any) any {
	switch valueKind := value.(type) {
	case *Object:
		var mappedValues map[string]any
		if valueKind != nil && !valueKind.isWeak.Load() {
			mappedValues = make(map[string]any)
			valueKind.fields.Range(func(key, value any) bool {
				mappedValues[key.(string)] = toMap(value)
				return true
			})
		}
		return mappedValues
	case *List:
		var items []any
		if valueKind != nil {
			items = make([]any, 0, valueKind.Length())
			for _, item := range valueKind.All() {
				items = append(items, toMap(item))
			}
		}
		return items
	default:
		return valueKind
	}
}

func (o *Object) Set(field string, value any) {
	value = toObjectTypes(value)
	o.fields.Store(field, value)
}

func (o *Object) Get(field string) (any, bool) {
	return o.fields.Load(field)
}

func (o *Object) Delete(field string) {
	o.fields.Delete(field)
}

func (o *Object) GetObject(field string) (*Object, bool) {
	value, loaded := o.fields.Load(field)
	obj, ok := value.(*Object)
	return obj, loaded && ok
}

func (o *Object) EnsureObject(field string) (obj *Object, isObject bool) {
	value, _ := o.fields.LoadOrStore(field, newWeakObject())
	obj, ok := value.(*Object)
	return obj, ok
}

func (o *Object) GetList(field string) (*List, bool) {
	value, loaded := o.fields.Load(field)
	list, ok := value.(*List)
	return list, loaded && ok
}

func (o *Object) EnsureList(field string) (list *List, isList bool) {
	value, _ := o.fields.LoadOrStore(field, NewList(nil))
	list, ok := value.(*List)
	return list, ok
}

func (o *Object) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.ToMap())
}
