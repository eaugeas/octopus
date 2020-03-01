package interval

import (
	"math"

	"github.com/tlblanc/octopus/container/tree"
)

// IntLesser is an implementor of tree.Lesser that
// can be used to compare intervals within a tree.
// It is based on the comparison of only the minimum
// number of the interval
type IntLesser struct{}

func (IntLesser) Less(a, b interface{}) int {
	amin := a.(Int).Min()
	bmin := b.(Int).Min()
	if amin < bmin {
		return -1
	} else if amin == bmin {
		return 0
	} else {
		return 1
	}
}

// Int represents an interval with integers. An interval
// is represented by two integers a, b such that
// [a, b]. An interval is immutable.
type Int struct {
	min int
	max int
}

// NewInt returns a new interval
func NewInt(min, max int) Int {
	if min > max {
		panic("min cannot be greater than max")
	}

	return Int{min: min, max: max}
}

// Min returns the a of the interval [a, b]
func (i Int) Min() int {
	return i.min
}

// Max returns the b of the interval [a, b]
func (i Int) Max() int {
	return i.max
}

// Len returns the length of the interval
func (i Int) Len() int {
	return i.max - i.min + 1
}

// Contains returns true if the interval represented
// by j is contained by i
func (i *Int) Contains(j Int) bool {
	return i.min <= j.min && j.max <= i.max
}

// Disjoints returns true if the intersection between
// i and j is empty. All empty intervals are disjoint
func (i Int) Disjoints(j Int) bool {
	return (i.min < j.min && i.max < j.min) ||
		(j.min < i.min && j.max < i.min)
}

// Intersection returns the interval of intersection
// between i and j
func (i Int) Intersection(j Int) Int {
	if i.Disjoints(j) {
		panic("intersection between two disjoint intervals")
	}

	return Int{
		min: int(math.Max(float64(i.min), float64(j.min))),
		max: int(math.Min(float64(i.max), float64(j.max))),
	}
}

// CanMerge returns true if the both intervals can be
// merged into one. That is, if i and j are not disjoints
// or they share a boundary. For example, i = [a, b] and
// j = [b + 1, c], in which case the resulting merged
// interval would be k = [a, c]
func (i Int) CanMerge(j Int) bool {
	return !i.Disjoints(j) || i.min == j.max+1 || i.max+1 == j.min
}

// Merge merges two intervals and returns the result
// in a new interval. Only a pair of non disjoints
// intervals can be merged. If j is disjoint with i
// Merge will panic
func (i Int) Merge(j Int) Int {
	if !i.CanMerge(j) {
		panic("cannot merge intervals")
	}

	return Int{
		min: int(math.Min(float64(i.min), float64(j.min))),
		max: int(math.Max(float64(i.max), float64(j.max))),
	}
}

// IntSet represents a set of intervals
// of the type {[Ii.Min(), Ii.Max()], i = 0 .. Len()}, where
// the intervals are disjoints. A Window is a
// useful data structure to keep track of continuous
// sets of objects.
//
// A use case for this is to keep track of message offsets
// that are continous. Instead of keeping [1, 2, 3, 5],
// for example, a window can keep [1, 3], [5] and when 4
// is added to the window, the result will be [1, 5],
// instead of [1, 2, 3, 4, 5].
type IntSet struct {
	intervals *tree.Tree
}

// NewIntSet creates a new instance of a interval set
func NewIntSet() *IntSet {
	return &IntSet{intervals: tree.NewRedBlackTree(IntLesser{})}
}

// Len returns the number of disjoint intervals
func (s *IntSet) Len() int {
	return s.intervals.Len()
}

// Contains returns true if the set contains
// any interval which contains the interval
func (s *IntSet) Contains(i Int) bool {
	lowerNode := s.intervals.Lower(i)
	if lowerNode == nil {
		return false
	}

	lower := lowerNode.Value.(Int)
	return lower.Min() <= i.Min() && i.Max() <= lower.Max()
}

// Insert inserts an interval to the set. If there already
// is an interval in the tree which is not disjoint with i
// the two will be merged
func (s *IntSet) Insert(i Int) {
	if lower, ok := s.lower(i); ok && i.CanMerge(lower) {
		ok := s.intervals.Delete(lower)
		if !ok {
			panic("failed to delete lower node")
		}

		i = i.Merge(lower)
	}

	for {
		if higher, ok := s.higher(i); ok && i.CanMerge(higher) {
			ok := s.intervals.Delete(higher)
			if !ok {
				panic("failed to delete higher node")
			}

			i = i.Merge(higher)
		} else {
			break
		}
	}

	s.intervals.Insert(i)
}

func (s *IntSet) higher(i Int) (Int, bool) {
	node := s.intervals.Higher(i)
	if node == nil {
		return Int{0, 0}, false
	}

	return node.Value.(Int), true
}

func (s *IntSet) lower(i Int) (Int, bool) {
	node := s.intervals.Lower(i)
	if node == nil {
		return Int{0, 0}, false
	}

	return node.Value.(Int), true
}
