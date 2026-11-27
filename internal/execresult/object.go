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

// Object represents an execution result value. It's raw form is equivalent to map[string]any.
// Objects may be set to their "weak" form, which allows them to marshal to 'null' when returned from Gateway.
type Object struct {
	// lazy holds the fields of an object nothing has read or written yet. They move into
	// fields on first use, so a value no step looks at is never converted or copied.
	lazy   atomic.Pointer[map[string]any]
	lazyMu sync.Mutex
	fields sync.Map
	isWeak atomic.Bool
}

// NewObject returns a new, strong [Object] with zero fields
func NewObject() *Object {
	return &Object{}
}

// NewObjectFromMap returns a new, strong [Object] to represent the given map m
func NewObjectFromMap(m map[string]any) *Object {
	obj := toObjectTypes(m).(*Object)
	// Callers go on to change m, so the Object must not keep it.
	obj.materialize()
	return obj
}

func toObjectTypes(v any) any {
	switch v := v.(type) {
	case map[string]any:
		if v == nil {
			return newWeakObject()
		}
		obj := NewObject()
		obj.lazy.Store(&v)
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

// materialize moves the fields of o out of lazy, converting them one level deep.
func (o *Object) materialize() {
	if o.lazy.Load() == nil {
		return
	}
	o.lazyMu.Lock()
	defer o.lazyMu.Unlock()
	lazy := o.lazy.Load()
	if lazy == nil {
		return
	}
	for key, value := range *lazy {
		o.fields.Store(key, toObjectTypes(value))
	}
	// Readers skip the lock once lazy is nil, so clear it only after every field is in place.
	o.lazy.Store(nil)
}

func newWeakObject() *Object {
	o := NewObject()
	o.SetWeak()
	return o
}

// SetWeak changes o to a weak reference. Marshaling o to JSON will return 'null'.
func (o *Object) SetWeak() {
	o.isWeak.Store(true)
}

// MergeOverrides merges overrides into o.
//
// If o is weak and overrides is strong, o's fields are replaced by overrides and set to a strong reference.
// If o is weak and overrides is nil or weak, then o's fields are replaced by override's fields and set to a weak reference.
func (o *Object) MergeOverrides(overrides *Object) {
	o.materialize()
	if o.isWeak.CompareAndSwap(true, false) {
		o.fields.Clear()
		if overrides == nil || overrides.isWeak.Load() {
			// When overrides is nil, then this object should become 'null' when marshaling to a map.
			// Weak overrides are assumed to be empty (nil map) and also become 'null'.
			o.isWeak.Store(true)
		}
	}
	if overrides == nil {
		return
	}
	overrides.materialize()
	overrides.fields.Range(func(key, value any) bool {
		o.set(key.(string), value)
		return true
	})
}

// ToMap returns the JSON-like map and array representation of o.
// Objects are marshaled as map[string]any, Lists as []any, and other values unchanged.
// A value nothing has read or written is returned as it was given, not as a copy.
func (o *Object) ToMap() map[string]any {
	return toMap(o).(map[string]any)
}

// String implements [fmt.Stringer] for easy debugging
func (o *Object) String() string {
	value := o.ToMap()
	s := fmt.Sprint(value)
	if value == nil {
		s = "map(nil)"
	}
	if o.isWeak.Load() {
		s += "(weak)"
	}
	return s
}

func toMap(value any) any {
	switch valueKind := value.(type) {
	case *Object:
		var mappedValues map[string]any
		if valueKind != nil {
			var newValues map[string]any
			if lazy := valueKind.lazy.Load(); lazy != nil {
				newValues = *lazy
			} else {
				newValues = make(map[string]any)
				valueKind.fields.Range(func(key, value any) bool {
					newValues[key.(string)] = toMap(value)
					return true
				})
			}
			if !valueKind.isWeak.Load() || len(newValues) > 0 {
				mappedValues = newValues
			}
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

// set sets o's field 'field' to the object type of 'value'.
// Set makes no attempt to consider strong or weak, use carefully.
func (o *Object) set(field string, value any) {
	o.materialize()
	value = toObjectTypes(value)
	o.fields.Store(field, value)
}

// Get returns o's value for field and true, or nil and false if not set
func (o *Object) Get(field string) (any, bool) {
	o.materialize()
	return o.fields.Load(field)
}

// Delete deletes o's field 'field' if it exists
func (o *Object) Delete(field string) {
	o.materialize()
	o.fields.Delete(field)
}

// GetObject returns o's field 'field' as an object and true if it exists and is an [Object].
// Returns nil and false otherwise.
func (o *Object) GetObject(field string) (*Object, bool) {
	o.materialize()
	value, loaded := o.fields.Load(field)
	obj, ok := value.(*Object)
	return obj, loaded && ok
}

// EnsureObject returns o's field 'field' as an object or stores and returns a new weak object.
// Returns false if the value exists and is not an object.
func (o *Object) EnsureObject(field string) (obj *Object, isObject bool) {
	o.materialize()
	value, _ := o.fields.LoadOrStore(field, newWeakObject())
	obj, ok := value.(*Object)
	return obj, ok
}

// GetList returns o's field 'field' as a list and true if it exists and is a [List].
// Returns nil and false otherwise.
func (o *Object) GetList(field string) (*List, bool) {
	o.materialize()
	value, loaded := o.fields.Load(field)
	list, ok := value.(*List)
	return list, loaded && ok
}

// EnsureList returns o's field 'field' as a list or stores and returns a new list.
// Returns false if the value exists and is not a list.
func (o *Object) EnsureList(field string) (list *List, isList bool) {
	o.materialize()
	value, _ := o.fields.LoadOrStore(field, NewList(nil))
	list, ok := value.(*List)
	return list, ok
}

// MarshalJSON implements [json.Marshaler]
func (o *Object) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.ToMap())
}
