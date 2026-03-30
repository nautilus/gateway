package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUniqueSet(t *testing.T) {
	t.Parallel()
	twoValuesSlice := []int{1, 2}
	twoValues := newSet(twoValuesSlice)

	assert.EqualValues(t, map[int]struct{}{
		1: {},
		2: {},
	}, twoValues)
	assert.ElementsMatch(t, twoValuesSlice, twoValues.ToSlice())

	oneValue := newSet([]int{1})

	assert.Equal(t, oneValue, twoValues.Intersection(oneValue))
	assert.Equal(t, oneValue, oneValue.Intersection(twoValues))

	assert.False(t, oneValue.Equal(twoValues))
	assert.True(t, twoValues.Equal(twoValues))
}
