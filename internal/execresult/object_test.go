package execresult

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewObject(t *testing.T) {
	t.Parallel()
	assert.Equal(t, &Object{}, NewObject())

	assert.Equal(t, newWeakObject(), NewObjectFromMap(nil))

	mapValue := map[string]any{
		"foo": true,
		"bar": 1,
		"baz": "baz",
		"biff": map[string]any{
			"boo": map[string]any{
				"boo": "boo",
			},
		},
		"bah": []any{
			"bah",
			1,
			true,
		},
		"humbug":         NewObject(),
		"woop":           NewList(nil),
		"nil":            nil,
		"map-typed nil":  map[string]any(nil),
		"list-typed nil": []any(nil),
	}
	expected := map[string]any{
		"foo": true,
		"bar": 1,
		"baz": "baz",
		"biff": map[string]any{
			"boo": map[string]any{
				"boo": "boo",
			},
		},
		"bah": []any{
			"bah",
			1,
			true,
		},
		"humbug":         map[string]any{},
		"woop":           []any{},
		"nil":            nil,
		"map-typed nil":  map[string]any(nil),
		"list-typed nil": nil,
	}
	require.Equal(t, expected, NewObjectFromMap(mapValue).ToMap())
}

func TestToMap_niladic_values(t *testing.T) {
	t.Parallel()
	assert.Equal(t, nil, toMap(nil))
	assert.Equal(t, map[string]any(nil), toMap((*Object)(nil)))
	assert.Equal(t, []any(nil), toMap((*List)(nil)))
}

func newAtomicBool(value bool) *atomic.Bool {
	var b atomic.Bool
	b.Store(value)
	return &b
}

func TestWeakObject(t *testing.T) {
	t.Parallel()
	t.Run("default to strong reference object", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, &Object{isWeak: *newAtomicBool(false)}, NewObject())
	})

	t.Run("can set to weak reference", func(t *testing.T) {
		t.Parallel()
		obj := NewObject()
		obj.SetWeak()
		assert.Equal(t, &Object{isWeak: *newAtomicBool(true)}, obj)
	})

	t.Run("new weak object behaves the same way", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, &Object{isWeak: *newAtomicBool(true)}, newWeakObject())
	})
}

func TestObject_MergeOverrides(t *testing.T) {
	t.Parallel()
	t.Run("strong", func(t *testing.T) {
		t.Parallel()
		obj := NewObjectFromMap(map[string]any{
			"foo": "bar",
			"baz": "biff",
		})
		obj.MergeOverrides(NewObjectFromMap(map[string]any{
			"foo": "boo",
		}))
		assert.Equal(t, map[string]any{
			"foo": "boo",
			"baz": "biff",
		}, obj.ToMap())
	})

	t.Run("weak self", func(t *testing.T) {
		t.Parallel()
		obj := NewObjectFromMap(map[string]any{
			"foo": "bar",
			"baz": "biff",
		})
		obj.SetWeak()
		obj.MergeOverrides(NewObjectFromMap(map[string]any{
			"foo": "boo",
		}))
		assert.Equal(t, map[string]any{
			"foo": "boo",
		}, obj.ToMap())
	})

	t.Run("weak self and nil overrides", func(t *testing.T) {
		t.Parallel()
		obj := NewObjectFromMap(map[string]any{
			"foo": "bar",
			"baz": "biff",
		})
		obj.SetWeak()
		obj.MergeOverrides(nil) // nil overrides can come from a failed query (had an error)
		assert.Equal(t, map[string]any(nil), obj.ToMap())
		assert.True(t, obj.isWeak.Load(), "Merging a nil override must result in a weak null value, so future strong merges succeed")
	})

	t.Run("weak self and weak empty other", func(t *testing.T) {
		t.Parallel()
		obj := NewObjectFromMap(map[string]any{
			"foo": "bar",
			"baz": "biff",
		})
		obj.SetWeak()
		other := newWeakObject()
		obj.MergeOverrides(other)
		assert.Equal(t, map[string]any(nil), obj.ToMap())
		assert.True(t, obj.isWeak.Load(), "Merging weak into weak must remain weak")
	})

	t.Run("nested objects", func(t *testing.T) {
		t.Parallel()
		obj := NewObjectFromMap(map[string]any{
			"foo": map[string]any{
				"bar": "baz",
			},
		})
		obj.MergeOverrides(NewObjectFromMap(map[string]any{
			"foo": "biff",
			"boo": map[string]any{
				"bah": "bam",
			},
		}))
		_, ok := obj.GetObject("boo")
		assert.True(t, ok, "nested value is an Object now")
		assert.Equal(t, map[string]any{
			"foo": "biff",
			"boo": map[string]any{
				"bah": "bam",
			},
		}, obj.ToMap())
	})
}

func TestObject_Gets(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"foo": 1,
		"bar": map[string]any{
			"baz": 2,
		},
		"biff": []any{
			"boo",
		},
	}
	obj := NewObjectFromMap(data)

	t.Run("get basic value", func(t *testing.T) {
		t.Parallel()
		t.Run("found", func(t *testing.T) {
			t.Parallel()
			value, ok := obj.Get("foo")
			assert.True(t, ok)
			assert.Equal(t, 1, value)
		})

		t.Run("not found", func(t *testing.T) {
			t.Parallel()
			value, ok := obj.Get("not found")
			assert.False(t, ok)
			assert.Nil(t, value)
		})
	})

	t.Run("get object", func(t *testing.T) {
		t.Parallel()
		t.Run("wrong type", func(t *testing.T) {
			t.Parallel()
			subObject, ok := obj.GetObject("foo")
			assert.False(t, ok)
			assert.Nil(t, subObject)
		})

		t.Run("found", func(t *testing.T) {
			t.Parallel()
			subObject, ok := obj.GetObject("bar")
			assert.True(t, ok)
			assert.Equal(t, map[string]any{"baz": 2}, subObject.ToMap())
		})

		t.Run("not found", func(t *testing.T) {
			t.Parallel()
			subObject, ok := obj.GetObject("not found")
			assert.False(t, ok)
			assert.Nil(t, subObject)
		})
	})

	t.Run("get list", func(t *testing.T) {
		t.Parallel()
		t.Run("wrong type", func(t *testing.T) {
			t.Parallel()
			list, ok := obj.GetList("foo")
			assert.False(t, ok)
			assert.Nil(t, list)
		})

		t.Run("found", func(t *testing.T) {
			t.Parallel()
			list, ok := obj.GetList("biff")
			assert.True(t, ok)

			var items []any
			for _, item := range list.All() {
				items = append(items, item)
			}
			assert.Equal(t, []any{"boo"}, items)
		})

		t.Run("not found", func(t *testing.T) {
			t.Parallel()
			list, ok := obj.GetList("not found")
			assert.False(t, ok)
			assert.Nil(t, list)
		})
	})
}

func TestObject_SetsAndEnsures(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"foo": 1,
		"bar": map[string]any{
			"baz": 2,
		},
		"biff": []any{
			"boo",
		},
	}
	obj := NewObjectFromMap(data)

	t.Run("set", func(t *testing.T) {
		t.Parallel()
		t.Run("single set", func(t *testing.T) {
			t.Parallel()
			obj.Set("set", 1)
			value, ok := obj.Get("set")
			assert.True(t, ok)
			assert.Equal(t, 1, value)
		})

		t.Run("single set nested values", func(t *testing.T) {
			t.Parallel()
			obj.Set("set nested 1", map[string]any{
				"foo": "bar",
			})
			obj.Set("set nested 2", []any{"foo"})
			_, ok := obj.GetObject("set nested 1")
			assert.True(t, ok)
			_, ok = obj.GetList("set nested 2")
			assert.True(t, ok)
		})

		t.Run("concurrent set", func(t *testing.T) {
			t.Parallel()
			var wait sync.WaitGroup
			const concurrentRoutines = 10
			for range concurrentRoutines {
				wait.Go(func() {
					obj.Set("set concurrent", 1)
					value, ok := obj.Get("set concurrent")
					assert.True(t, ok)
					assert.Equal(t, 1, value)
				})
			}
			wait.Wait()
		})
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()
		obj.Set("deletable", 1)
		value, ok := obj.Get("deletable")
		assert.True(t, ok)
		assert.Equal(t, 1, value)

		obj.Delete("deletable")
		value, ok = obj.Get("deletable")
		assert.False(t, ok)
		assert.Nil(t, value)
	})

	t.Run("ensure object", func(t *testing.T) {
		t.Parallel()
		t.Run("wrong type", func(t *testing.T) {
			t.Parallel()
			subObject, ok := obj.EnsureObject("foo")
			assert.False(t, ok)
			assert.Nil(t, subObject)
		})

		t.Run("found", func(t *testing.T) {
			t.Parallel()
			subObject, ok := obj.EnsureObject("bar")
			assert.True(t, ok)
			assert.Equal(t, map[string]any{"baz": 2}, subObject.ToMap())
		})

		t.Run("not found", func(t *testing.T) {
			t.Parallel()
			subObject, ok := obj.EnsureObject("not found object")
			assert.True(t, ok)
			assert.Equal(t, newWeakObject(), subObject)
		})

		t.Run("concurrent not found", func(t *testing.T) {
			t.Parallel()
			var wait sync.WaitGroup
			const concurrentRoutines = 10
			for range concurrentRoutines {
				wait.Go(func() {
					subObject, ok := obj.EnsureObject("not found object concurrent")
					assert.True(t, ok)
					assert.Equal(t, newWeakObject(), subObject)
				})
			}
			wait.Wait()
		})
	})

	t.Run("ensure list", func(t *testing.T) {
		t.Parallel()
		t.Run("wrong type", func(t *testing.T) {
			t.Parallel()
			list, ok := obj.EnsureList("foo")
			assert.False(t, ok)
			assert.Nil(t, list)
		})

		t.Run("found", func(t *testing.T) {
			t.Parallel()
			list, ok := obj.EnsureList("biff")
			assert.True(t, ok)

			var items []any
			for _, item := range list.All() {
				items = append(items, item)
			}
			assert.Equal(t, []any{"boo"}, items)
		})

		t.Run("not found", func(t *testing.T) {
			t.Parallel()
			list, ok := obj.EnsureList("not found list")
			assert.True(t, ok)
			assert.Equal(t, 0, list.Length())
		})

		t.Run("concurrent not found", func(t *testing.T) {
			t.Parallel()
			var wait sync.WaitGroup
			const concurrentRoutines = 10
			for range concurrentRoutines {
				wait.Go(func() {
					list, ok := obj.EnsureList("not found list concurrent")
					assert.True(t, ok)
					assert.Equal(t, 0, list.Length())
				})
			}
			wait.Wait()
		})
	})
}

func TestObject_MarshalJSON(t *testing.T) {
	t.Parallel()
	value, err := NewObjectFromMap(map[string]any{
		"foo": "foo",
		"bar": 1,
		"baz": map[string]any{
			"biff": "biff",
		},
		"boo": []any{true},
	}).MarshalJSON()
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"foo": "foo",
		"bar": 1,
		"baz": {
			"biff": "biff"
		},
		"boo": [true]
	}`, string(value))
}

func TestObject_String(t *testing.T) {
	t.Parallel()
	value := map[string]any{"foo": "bar"}
	assert.Equal(t, fmt.Sprint(value), NewObjectFromMap(value).String())

	assert.Equal(t, "map(nil)(weak)", newWeakObject().String())
}
