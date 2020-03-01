package interval

import (
	"container/heap"
	"testing"

	"github.com/stretchr/testify/assert"
)

// An IntHeap is a min-heap of ints. A heap is a useful
// data structure to compare benchmarks against
type IntHeap []Int

func (h IntHeap) Len() int           { return len(h) }
func (h IntHeap) Less(i, j int) bool { return h[i].min < h[j].min }
func (h IntHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *IntHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, x.(Int))
}

func (h *IntHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func TestIntNewOK(t *testing.T) {
	i := NewInt(1, 5)

	assert.Equal(t, 5, i.Len())
	assert.Equal(t, 1, i.Min())
	assert.Equal(t, 5, i.Max())
}

func TestIntNewErrPanics(t *testing.T) {
	assert.Panics(t, func() {
		NewInt(0, -1)
	})
}

func TestIntContainsOK(t *testing.T) {
	i := NewInt(1, 5)

	assert.True(t, i.Contains(NewInt(1, 1)))
	assert.True(t, i.Contains(NewInt(1, 5)))
	assert.True(t, i.Contains(NewInt(3, 5)))

	assert.False(t, i.Contains(NewInt(0, 0)))
	assert.False(t, i.Contains(NewInt(0, 6)))
	assert.False(t, i.Contains(NewInt(3, 6)))
}

func TestIntDisjointsOK(t *testing.T) {
	i := NewInt(1, 5)

	assert.False(t, i.Disjoints(NewInt(1, 1)))
	assert.False(t, i.Disjoints(NewInt(1, 5)))
	assert.False(t, i.Disjoints(NewInt(3, 5)))
	assert.False(t, i.Disjoints(NewInt(0, 6)))
	assert.False(t, i.Disjoints(NewInt(3, 6)))

	assert.True(t, i.Disjoints(NewInt(0, 0)))
	assert.True(t, i.Disjoints(NewInt(6, 10)))
}

func TestIntIntersectionErrPanics(t *testing.T) {
	i := NewInt(1, 5)

	assert.Panics(t, func() {
		i.Intersection(NewInt(0, 0))
	})
}

func TestIntIntersectionOK(t *testing.T) {
	i := NewInt(1, 5)

	assert.Equal(t, NewInt(2, 4), i.Intersection(NewInt(2, 4)))
	assert.Equal(t, NewInt(3, 5), i.Intersection(NewInt(3, 5)))
	assert.Equal(t, NewInt(3, 5), i.Intersection(NewInt(3, 10)))
	assert.Equal(t, NewInt(5, 5), i.Intersection(NewInt(5, 5)))
}

func testIntMergeErrPanics(t *testing.T) {
	i := NewInt(1, 5)

	assert.Panics(t, func() {
		i.Merge(NewInt(-1, -1))
	})
}

func TestIntMergeOK(t *testing.T) {
	i := NewInt(1, 5)

	assert.Equal(t, NewInt(1, 5), i.Merge(NewInt(2, 4)))
	assert.Equal(t, NewInt(1, 10), i.Merge(NewInt(3, 10)))
	assert.Equal(t, NewInt(1, 8), i.Merge(NewInt(5, 8)))
	assert.Equal(t, NewInt(1, 8), i.Merge(NewInt(6, 8)))
	assert.Equal(t, NewInt(1, 8), i.Merge(NewInt(5, 8)))
	assert.Equal(t, NewInt(-1, 5), i.Merge(NewInt(-1, 0)))
}

func TestIntSetContainsOKSingle(t *testing.T) {
	s := NewIntSet()

	s.Insert(NewInt(1, 3))

	assert.True(t, s.Contains(NewInt(1, 1)))
	assert.True(t, s.Contains(NewInt(2, 2)))
	assert.True(t, s.Contains(NewInt(3, 3)))
	assert.True(t, s.Contains(NewInt(1, 3)))

	assert.False(t, s.Contains(NewInt(0, 0)))
	assert.False(t, s.Contains(NewInt(4, 4)))
	assert.False(t, s.Contains(NewInt(0, 4)))
	assert.False(t, s.Contains(NewInt(4, 10)))
}

func TestIntSetContainsOKMultiple(t *testing.T) {
	s := NewIntSet()

	s.Insert(NewInt(1, 3))
	s.Insert(NewInt(7, 10))
	s.Insert(NewInt(13, 20))

	assert.True(t, s.Contains(NewInt(1, 2)))
	assert.True(t, s.Contains(NewInt(8, 10)))
	assert.True(t, s.Contains(NewInt(14, 19)))

	assert.False(t, s.Contains(NewInt(3, 5)))
	assert.False(t, s.Contains(NewInt(4, 6)))
	assert.False(t, s.Contains(NewInt(5, 7)))
	assert.False(t, s.Contains(NewInt(7, 11)))
	assert.False(t, s.Contains(NewInt(12, 21)))
	assert.False(t, s.Contains(NewInt(100, 200)))
}

func TestIntSetInsertOKDisjoints(t *testing.T) {
	s := NewIntSet()
	assert.Equal(t, 0, s.Len())

	s.Insert(NewInt(1, 3))
	assert.Equal(t, 1, s.Len())

	s.Insert(NewInt(5, 7))
	assert.Equal(t, 2, s.Len())

	assert.True(t, s.Contains(NewInt(1, 3)))
	assert.True(t, s.Contains(NewInt(5, 7)))
}

func TestIntSetInsertOKMergeOne(t *testing.T) {
	s := NewIntSet()
	assert.Equal(t, 0, s.Len())

	s.Insert(NewInt(1, 3))
	assert.Equal(t, 1, s.Len())

	s.Insert(NewInt(2, 5))
	assert.Equal(t, 1, s.Len())

	s.Insert(NewInt(5, 7))
	assert.Equal(t, 1, s.Len())

	s.Insert(NewInt(8, 10))
	assert.Equal(t, 1, s.Len())

	assert.True(t, s.Contains(NewInt(1, 10)))
	assert.False(t, s.Contains(NewInt(1, 11)))
	assert.False(t, s.Contains(NewInt(0, 10)))
}

func TestIntSetInsertOKMergeAll(t *testing.T) {
	s := NewIntSet()
	assert.Equal(t, 0, s.Len())

	s.Insert(NewInt(1, 3))
	s.Insert(NewInt(5, 7))
	s.Insert(NewInt(9, 12))
	s.Insert(NewInt(15, 20))
	s.Insert(NewInt(25, 30))
	s.Insert(NewInt(35, 40))
	assert.Equal(t, 6, s.Len())

	s.Insert(NewInt(2, 38))
	assert.Equal(t, 1, s.Len())

	assert.True(t, s.Contains(NewInt(1, 40)))
	assert.False(t, s.Contains(NewInt(1, 41)))
	assert.False(t, s.Contains(NewInt(0, 40)))
}

func BenchmarkIntSetAddSeq(b *testing.B) {
	s := NewIntSet()
	for i := 0; i < b.N; i++ {
		s.Insert(NewInt(i, i))
	}

	assert.Equal(b, 1, s.Len())
	assert.True(b, s.Contains(NewInt(0, b.N-1)))
}

func BenchmarkHeapAddSeq(b *testing.B) {
	h := &IntHeap{}
	heap.Init(h)

	for i := 0; i < b.N; i++ {
		heap.Push(h, NewInt(i, i))
	}
}
