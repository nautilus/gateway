package execresult

import (
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collect[First, Second any](seq2 iter.Seq2[First, Second]) ([]First, []Second) {
	var first []First
	var second []Second
	for a, b := range seq2 {
		first = append(first, a)
		second = append(second, b)
	}
	return first, second
}

func TestList_NewAndAll(t *testing.T) {
	t.Parallel()
	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 0, NewList(nil).Length())
	})

	t.Run("basic values", func(t *testing.T) {
		t.Parallel()
		values := []any{"a", "b", "c"}
		list := NewList(values)
		assert.Equal(t, 3, list.Length())
		indices, items := collect(list.All())
		assert.Equal(t, []int{0, 1, 2}, indices)
		assert.Equal(t, values, items)
	})

	t.Run("basic values stop iterating early", func(t *testing.T) {
		t.Parallel()
		list := NewList([]any{"a", "b", "c"})
		var items []any
		for index, value := range list.All() {
			if index == 2 {
				break
			}
			items = append(items, value)
		}
		assert.Equal(t, []any{"a", "b"}, items)
	})

	t.Run("nested values", func(t *testing.T) {
		t.Parallel()
		values := []any{
			true,
			map[string]any{
				"foo": 1,
			},
			[]any{
				"biff",
			},
		}
		list := NewList(values)
		_, items := collect(list.All())
		require.Len(t, items, 3)
		item0, item1, item2 := items[0], items[1].(*Object), items[2].(*List)
		assert.Equal(t, true, item0)
		assert.Equal(t, map[string]any{"foo": 1}, item1.ToMap())

		_, subListItems := collect(item2.All())
		assert.Equal(t, []any{"biff"}, subListItems)
	})
}

func TestList_EnsureObjectAtIndex(t *testing.T) {
	t.Parallel()
	list := NewList([]any{
		1,
		map[string]any{
			"foo": 2,
		},
	})
	value, ok := list.EnsureObjectAtIndex(0)
	assert.False(t, ok)
	assert.Nil(t, value)
	assert.Equal(t, 2, list.Length(), "size does not change")

	value, ok = list.EnsureObjectAtIndex(1)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"foo": 2}, value.ToMap())
	assert.Equal(t, 2, list.Length(), "size does not change")

	value, ok = list.EnsureObjectAtIndex(2)
	require.True(t, ok)
	assert.Equal(t, newWeakObject(), value)
	assert.Equal(t, 3, list.Length(), "size should grow")
}

func TestList_Get(t *testing.T) {
	t.Parallel()
	list := NewList([]any{
		1,
		map[string]any{
			"foo": 2,
		},
	})
	value, ok := list.Get(-1)
	assert.False(t, ok)
	assert.Nil(t, value)

	value, ok = list.Get(0)
	assert.True(t, ok)
	assert.Equal(t, 1, value)
	value, ok = list.GetObjectAtIndex(0)
	assert.False(t, ok)
	assert.Nil(t, value)
	assert.Equal(t, 2, list.Length(), "size does not change")

	value, ok = list.Get(1)
	assert.True(t, ok)
	require.IsType(t, (*Object)(nil), value)
	assert.Equal(t, map[string]any{"foo": 2}, value.(*Object).ToMap())
	objValue, ok := list.GetObjectAtIndex(1)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"foo": 2}, objValue.ToMap())
	assert.Equal(t, 2, list.Length(), "size does not change")

	value, ok = list.Get(2)
	assert.False(t, ok)
	assert.Nil(t, value)
	value, ok = list.GetObjectAtIndex(2)
	assert.False(t, ok)
	assert.Nil(t, value)
	assert.Equal(t, 2, list.Length(), "size does not change")
}

func TestList_MarshalJSON(t *testing.T) {
	t.Parallel()
	value, err := NewList([]any{
		"foo",
		1,
		map[string]any{
			"biff": "biff",
		},
		[]any{true},
	}).MarshalJSON()
	require.NoError(t, err)
	assert.JSONEq(t, `[
		"foo",
		1,
		{ "biff": "biff" },
		[true]
	]`, string(value))
}
