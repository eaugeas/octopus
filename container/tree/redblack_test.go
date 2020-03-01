package tree

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func prePopulatedRedBlackTree() *Tree {
	tree := NewRedBlackTree(IntLesser{})
	prePopulateTree(tree)
	return tree
}

func TestRedBlackTreeRootNil(t *testing.T) {
	tree := NewRedBlackTree(IntLesser{})
	assert.Nil(t, tree.Root())
	assert.Equal(t, 0, tree.Len())
}

func TestRedBlackTreeRootNode(t *testing.T) {
	tree := NewRedBlackTree(IntLesser{})
	tree.Insert(1)
	assert.Equal(t, 1, tree.Root().Value)
	assert.Equal(t, 1, tree.Len())
}

func TestRedBlackTreeInsertBalanced(t *testing.T) {
	tree := NewRedBlackTree(IntLesser{})

	tree.Insert(1)
	tree.Insert(0)
	tree.Insert(2)

	assertEqualTree(t, [][]interface{}{
		[]interface{}{1},
		[]interface{}{0, 2},
	}, tree)
}

func TestRedBlackTreeInsertMultiple(t *testing.T) {
	tree := NewRedBlackTree(IntLesser{})

	for i := 0; i < 4; i++ {
		tree.Insert(i)
	}

	assertEqualTree(t, [][]interface{}{
		[]interface{}{1},
		[]interface{}{0, 2},
		[]interface{}{nil, nil, nil, 3},
	}, tree)
}

func TestRedBlackTreeEmptyInOrderWalk(t *testing.T) {
	tree := NewRedBlackTree(IntLesser{})
	called := 0
	tree.InOrderWalk(func(n *Node) {
		called++
	})

	assert.Equal(t, 0, called)
}

func TestRedBlackTreeInOrderWalkOneLevel(t *testing.T) {
	tree := NewRedBlackTree(IntLesser{})
	var res []int

	tree.Insert(1)
	tree.Insert(0)
	tree.Insert(2)

	tree.InOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{0, 1, 2}, res)
}

func TestRedBlackTreePostOrderWalkOneLevel(t *testing.T) {
	tree := NewRedBlackTree(IntLesser{})
	var res []int

	tree.Insert(1)
	tree.Insert(0)
	tree.Insert(2)

	tree.PostOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{2, 1, 0}, res)
}

func TestRedBlackTreePreOrderWalkOneLevel(t *testing.T) {
	tree := NewRedBlackTree(IntLesser{})
	var res []int

	tree.Insert(1)
	tree.Insert(0)
	tree.Insert(2)

	tree.PreOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{1, 0, 2}, res)
}

func TestRedBlackTreeInOrderWalkMultiLevel(t *testing.T) {
	tree := prePopulatedRedBlackTree()
	var res []int

	tree.InOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{0, 1, 1, 2, 3, 3, 5, 6, 7, 8}, res)
}

func TestRedBlackTreePostOrderWalkMultiLevel(t *testing.T) {
	tree := prePopulatedRedBlackTree()
	var res []int

	tree.PostOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{8, 7, 6, 5, 3, 3, 2, 1, 1, 0}, res)
}

func TestRedBlackTreePreOrderWalkMultiLevel(t *testing.T) {
	tree := prePopulatedRedBlackTree()
	var res []int

	tree.PreOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{5, 2, 1, 0, 1, 3, 3, 7, 6, 8}, res)
}

func TestRedBlackTreeMinOK(t *testing.T) {
	tree := prePopulatedRedBlackTree()

	n := tree.Min()

	assert.NotNil(t, n)
	assert.Equal(t, 0, n.Value)
}

func TestRedBlackTreeMinEmpty(t *testing.T) {
	tree := NewRedBlackTree(IntLesser{})
	n := tree.Min()
	assert.Nil(t, n)
}

func TestRedBlackTreeMaxOK(t *testing.T) {
	tree := prePopulatedRedBlackTree()

	n := tree.Max()

	assert.NotNil(t, n)
	assert.Equal(t, 8, n.Value)
}

func TestRedBlackTreeMaxEmpty(t *testing.T) {
	tree := NewRedBlackTree(IntLesser{})
	n := tree.Max()
	assert.Nil(t, n)
}

func TestRedBlackTreeFindOK(t *testing.T) {
	tree := prePopulatedRedBlackTree()

	n := tree.Find(2)

	assert.NotNil(t, n)
	assert.Equal(t, 2, n.Value.(int))
}

func TestRedBlackTreeFindNil(t *testing.T) {
	tree := prePopulatedRedBlackTree()
	n := tree.Find(1000)
	assert.Nil(t, n)
}

func TestRedBlackTreePredecessorOK(t *testing.T) {
	tree := prePopulatedRedBlackTree()

	n := tree.Find(2).Predecessor()

	assert.NotNil(t, n)
	assert.Equal(t, 1, n.Value)
}

func TestRedBlackTreeSuccessorOK(t *testing.T) {
	tree := prePopulatedRedBlackTree()

	n := tree.Find(2).Successor()

	assert.NotNil(t, n)
	assert.Equal(t, 3, n.Value)
}

func TestRedBlackTreeDeleteNoChildrenOK(t *testing.T) {
	tree := prePopulatedRedBlackTree()

	ok := tree.Delete(8)

	assert.True(t, ok)
	assertEqualTree(t, [][]interface{}{
		[]interface{}{5},
		[]interface{}{2, 7},
		[]interface{}{1, 3, 6, nil},
		[]interface{}{0, 1, nil, 3, nil, nil, nil, nil},
	}, tree)
}

func TestRedBlackTreeDeleteNoLeftChildrenOK(t *testing.T) {
	tree := prePopulatedRedBlackTree()

	ok := tree.Delete(3)

	assert.True(t, ok)
	assertEqualTree(t, [][]interface{}{
		[]interface{}{5},
		[]interface{}{2, 7},
		[]interface{}{1, 3, 6, 8},
		[]interface{}{0, 1, nil, nil, nil, nil, nil, nil},
	}, tree)
}

func TestRedBlackTreeDeleteTwoChildrenOK(t *testing.T) {
	tree := prePopulatedRedBlackTree()

	ok := tree.Delete(1)

	assert.True(t, ok)
	assertEqualTree(t, [][]interface{}{
		[]interface{}{5},
		[]interface{}{2, 7},
		[]interface{}{1, 3, 6, 8},
		[]interface{}{0, nil, nil, 3, nil, nil, nil, nil},
	}, tree)
}

func TestRedBlackTreeDeleteRootOK(t *testing.T) {
	tree := prePopulatedRedBlackTree()

	ok := tree.Delete(5)
	assert.True(t, ok)

	assertEqualTree(t, [][]interface{}{
		[]interface{}{6},
		[]interface{}{2, 7},
		[]interface{}{1, 3, nil, 8},
		[]interface{}{0, 1, nil, 3, nil, nil, nil, nil},
	}, tree)
}

func TestRedBlackTreeDeleteAllOK(t *testing.T) {
	tree := prePopulatedRedBlackTree()

	for !tree.Empty() {
		ok := tree.Delete(tree.Root().Value)
		assert.True(t, ok)
	}

	assert.True(t, tree.Empty())
	assert.Equal(t, 0, tree.Len())
	assert.Nil(t, tree.Root())
}

func TestRedBlackTreeDeleteNotExistingNode(t *testing.T) {
	tree := prePopulatedRedBlackTree()

	ok := tree.Delete(100)
	assert.False(t, ok)
}

func BenchmarkRedBlackTreeNodeRandomInsert(b *testing.B) {
	tree := NewRedBlackTree(IntLesser{})

	for i := 0; i < b.N; i++ {
		tree.Insert(int(rand.Int31()))
	}
}

func BenchmarkRedBlackTreeGrowingSequenceInsert(b *testing.B) {
	tree := NewRedBlackTree(IntLesser{})

	for i := 0; i < b.N; i++ {
		tree.Insert(i)
	}
}

func BenchmarkRedBlackTreePreOrderSequenceInsert(b *testing.B) {
	tree := NewRedBlackTree(IntLesser{})
	gen := balancedTreeGenerator{Highest: uint(b.N << 1)}

	for i := 0; i < b.N; i++ {
		v, ok := gen.Next()
		if !ok {
			panic("generator failed to generate enough numbers")
		}
		tree.Insert(v)
	}
}

func BenchmarkRedBlackTreePreOrderSequenceInsertAndWalk(b *testing.B) {
	tree := NewRedBlackTree(IntLesser{})
	gen := balancedTreeGenerator{Highest: uint(b.N << 1)}
	count := 0

	for i := 0; i < b.N; i++ {
		v, ok := gen.Next()
		if !ok {
			panic("generator failed to generate enough numbers")
		}
		tree.Insert(v)
	}

	tree.InOrderWalk(func(n *Node) {
		count++
	})

	assert.Equal(b, b.N, count)
}
