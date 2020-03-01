package tree

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func prePopulatedUnbalancedTree() *Tree {
	tree := NewUnbalancedTree(IntLesser{})
	prePopulateTree(tree)
	return tree
}

func TestUnbalancedRootNil(t *testing.T) {
	tree := NewUnbalancedTree(IntLesser{})
	assert.Nil(t, tree.Root())
	assert.Equal(t, 0, tree.Len())
}

func TestUnbalancedRootNode(t *testing.T) {
	tree := NewUnbalancedTree(IntLesser{})
	tree.Insert(1)
	assert.Equal(t, 1, tree.Root().Value)
	assert.Equal(t, 1, tree.Len())
}

func TestUnbalancedTreeInsertBalanced(t *testing.T) {
	tree := NewUnbalancedTree(IntLesser{})

	tree.Insert(1)
	tree.Insert(0)
	tree.Insert(2)

	assertEqualTree(t, [][]interface{}{
		[]interface{}{1},
		[]interface{}{0, 2},
	}, tree)
}

func TestUnbalancedTreeInsert(t *testing.T) {
	tree := NewUnbalancedTree(IntLesser{})

	for i := 0; i < 4; i++ {
		tree.Insert(i)
	}

	assertEqualTree(t, [][]interface{}{
		[]interface{}{0},
		[]interface{}{nil, 1},
		[]interface{}{nil, nil, nil, 2},
		[]interface{}{nil, nil, nil, nil, nil, nil, nil, 3},
	}, tree)
}

func TestUnbalancedTreeNodeEmptyInOrderWalk(t *testing.T) {
	tree := NewUnbalancedTree(IntLesser{})
	called := 0
	tree.InOrderWalk(func(n *Node) {
		called++
	})

	assert.Equal(t, 0, called)
}

func TestUnbalancedTreeNodeInOrderWalkOneLevel(t *testing.T) {
	tree := NewUnbalancedTree(IntLesser{})
	var res []int

	tree.Insert(1)
	tree.Insert(0)
	tree.Insert(2)

	tree.InOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{0, 1, 2}, res)
}

func TestUnbalancedTreeNodePostOrderWalkOneLevel(t *testing.T) {
	tree := NewUnbalancedTree(IntLesser{})
	var res []int

	tree.Insert(1)
	tree.Insert(0)
	tree.Insert(2)

	tree.PostOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{2, 1, 0}, res)
}

func TestUnbalancedTreeNodePreOrderWalkOneLevel(t *testing.T) {
	tree := NewUnbalancedTree(IntLesser{})
	var res []int

	tree.Insert(1)
	tree.Insert(0)
	tree.Insert(2)

	tree.PreOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{1, 0, 2}, res)
}

func TestUnbalancedTreeNodeInOrderWalkMultiLevel(t *testing.T) {
	tree := prePopulatedUnbalancedTree()
	var res []int

	tree.InOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{0, 1, 1, 2, 3, 3, 5, 6, 7, 8}, res)
}

func TestUnbalancedTreeNodePostOrderWalkMultiLevel(t *testing.T) {
	tree := prePopulatedUnbalancedTree()
	var res []int

	tree.PostOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{8, 7, 6, 5, 3, 3, 2, 1, 1, 0}, res)
}

func TestUnbalancedTreeNodePreOrderWalkMultiLevel(t *testing.T) {
	tree := prePopulatedUnbalancedTree()
	var res []int

	tree.PreOrderWalk(func(n *Node) {
		res = append(res, n.Value.(int))
	})

	assert.Equal(t, []int{5, 2, 1, 0, 1, 3, 3, 7, 6, 8}, res)
}

func TestUnbalancedTreeMinOK(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	n := tree.Min()

	assert.NotNil(t, n)
	assert.Equal(t, 0, n.Value)
}

func TestUnbalancedTreeNodeMinEmpty(t *testing.T) {
	tree := NewUnbalancedTree(IntLesser{})
	n := tree.Min()
	assert.Nil(t, n)
}

func TestUnbalancedTreeNodeMaxOK(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	n := tree.Max()

	assert.NotNil(t, n)
	assert.Equal(t, 8, n.Value)
}

func TestUnbalancedTreeNodeMaxEmpty(t *testing.T) {
	tree := NewUnbalancedTree(IntLesser{})
	n := tree.Max()
	assert.Nil(t, n)
}

func TestUnbalancedTreeFindOK(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	n := tree.Find(2)

	assert.NotNil(t, n)
	assert.Equal(t, 2, n.Value.(int))
}

func TestUnbalancedTreeFindNil(t *testing.T) {
	tree := prePopulatedUnbalancedTree()
	n := tree.Find(1000)
	assert.Nil(t, n)
}

func TestUnbalancedTreeNodePredecessorOK(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	n := tree.Find(2).Predecessor()

	assert.NotNil(t, n)
	assert.Equal(t, 1, n.Value)
}

func TestUnbalancedTreeNodeSuccessorOK(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	n := tree.Find(2).Successor()

	assert.NotNil(t, n)
	assert.Equal(t, 3, n.Value)
}

func TestUnbalancedTreeNodeDeleteNoChildrenOK(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	ok := tree.Delete(8)

	assert.True(t, ok)
	assertEqualTree(t, [][]interface{}{
		[]interface{}{5},
		[]interface{}{2, 7},
		[]interface{}{1, 3, 6, nil},
		[]interface{}{0, 1, nil, 3, nil, nil, nil, nil},
	}, tree)
}

func TestUnbalancedTreeNodeDeleteNoLeftChildrenOK(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	ok := tree.Delete(3)

	assert.True(t, ok)
	assertEqualTree(t, [][]interface{}{
		[]interface{}{5},
		[]interface{}{2, 7},
		[]interface{}{1, 3, 6, 8},
		[]interface{}{0, 1, nil, nil, nil, nil, nil, nil},
	}, tree)
}

func TestUnbalancedTreeNodeDeleteTwoChildrenOK(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	ok := tree.Delete(1)

	assert.True(t, ok)
	assertEqualTree(t, [][]interface{}{
		[]interface{}{5},
		[]interface{}{2, 7},
		[]interface{}{1, 3, 6, 8},
		[]interface{}{0, nil, nil, 3, nil, nil, nil, nil},
	}, tree)
}

func TestUnbalancedTreeNodeDeleteRootOK(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	ok := tree.Delete(5)
	assert.True(t, ok)

	assertEqualTree(t, [][]interface{}{
		[]interface{}{6},
		[]interface{}{2, 7},
		[]interface{}{1, 3, nil, 8},
		[]interface{}{0, 1, nil, 3, nil, nil, nil, nil},
	}, tree)
}

func TestUnbalancedTreeNodeDeleteNotExistingNode(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	ok := tree.Delete(100)
	assert.False(t, ok)
}

func TestUnbalancedTreeHigher(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	n := tree.Higher(4)
	assert.Equal(t, 5, n.Value)
	n = tree.Higher(5)
	assert.Equal(t, 5, n.Value)
	n = tree.Higher(6)
	assert.Equal(t, 6, n.Value)
	n = tree.Higher(7)
	assert.Equal(t, 7, n.Value)
	n = tree.Higher(8)
	assert.Equal(t, 8, n.Value)
	n = tree.Higher(9)
	assert.Nil(t, n)
	n = tree.Higher(10)
	assert.Nil(t, n)
}

func TestUnbalancedTreeLower(t *testing.T) {
	tree := prePopulatedUnbalancedTree()

	n := tree.Lower(5)
	assert.Equal(t, 5, n.Value)
	n = tree.Lower(4)
	assert.Equal(t, 3, n.Value)
	n = tree.Lower(3)
	assert.Equal(t, 3, n.Value)
	n = tree.Lower(2)
	assert.Equal(t, 2, n.Value)
	n = tree.Lower(1)
	assert.Equal(t, 1, n.Value)
	n = tree.Lower(0)
	assert.Equal(t, 0, n.Value)
	n = tree.Lower(-1)
	assert.Nil(t, n)
}

func BenchmarkUnbalancedTreeNodeRandomInsert(b *testing.B) {
	tree := NewUnbalancedTree(IntLesser{})

	for i := 0; i < b.N; i++ {
		tree.Insert(int(rand.Int31()))
	}
}

func BenchmarkUnbalancedTreePreOrderSequenceInsert(b *testing.B) {
	tree := NewUnbalancedTree(IntLesser{})
	gen := balancedTreeGenerator{Highest: uint(b.N << 1)}

	for i := 0; i < b.N; i++ {
		v, ok := gen.Next()
		if !ok {
			panic("generator failed to generate enough numbers")
		}
		tree.Insert(v)
	}
}

func BenchmarkUnbalancedTreePreOrderSequenceInsertAndWalk(b *testing.B) {
	tree := NewUnbalancedTree(IntLesser{})
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
