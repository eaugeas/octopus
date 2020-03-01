package tree

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

const treeMaxValue = 10

type iterator interface {
	Next() (interface{}, bool)
}

type balancedTreeGenerator struct {
	level uint
	index uint

	// Highest sets the maximum value an element can have
	Highest uint
}

// Next is the implementation of Generator.Next for BalancedTreeGenerator
func (g *balancedTreeGenerator) Next() (int, bool) {
	if (math.Pow(2, float64(g.level)) + float64(g.index)) > float64(g.Highest) {
		return 0, false
	}

	levelElements := uint(math.Pow(2, float64(g.level)))
	value := (g.Highest * (2*g.index + 1)) / (2 * levelElements)

	g.index += 1
	if g.index >= levelElements {
		g.index = 0
		g.level += 1
	}

	return int(value), true
}

func levels(tree *Tree) [][]*Node {
	result := [][]*Node{[]*Node{nilIfSentinel(tree.root)}}
	currLevel := 0

	for {
		nels := int(math.Pow(2, float64(currLevel+1)))
		result = append(result, make([]*Node, nels))
		nodesAdded := 0

		for i := 0; i < nels/2; i++ {
			if result[currLevel][i] == nil {
				result[currLevel+1][2*i] = nil
				result[currLevel+1][2*i+1] = nil
			} else {
				nodesAdded += 1
				result[currLevel+1][2*i] = nilIfSentinel(result[currLevel][i].left)
				result[currLevel+1][2*i+1] = nilIfSentinel(result[currLevel][i].right)
			}
		}

		currLevel += 1
		if nodesAdded == 0 {
			break
		}
	}

	// the last level is empty so it can be removed
	return result[:currLevel-1]
}

func assertEqualTree(t *testing.T, expected [][]interface{}, tree *Tree) {
	levels := levels(tree)
	assert.Equal(t, len(expected), len(levels))
	for level := 0; level < len(expected); level++ {
		assert.Equal(t, len(expected[level]), len(levels[level]))
		for col := 0; col < len(expected[level]); col++ {
			if expected[level][col] == nil {
				assert.Nil(t, levels[level][col])
			} else {
				assert.NotNil(t, levels[level][col])
				assert.Equal(t, expected[level][col], levels[level][col].Value)
			}
		}
	}
}

func printTree(tree *Tree) {
	levels := levels(tree)

	fmt.Println()
	for _, level := range levels {
		for _, node := range level {
			if node == nil {

			} else {
				fmt.Printf(" %d ", node.Value.(int))
			}
		}
		fmt.Println()
	}
	fmt.Println()
}

func prePopulateTree(tree *Tree) {
	if tree.Len() != 0 {
		panic("attempt to prepopulate non-emtpy tree")
	}
	it := balancedTreeGenerator{Highest: treeMaxValue}
	for {
		value, ok := it.Next()
		if !ok {
			break
		}

		tree.Insert(value)
	}
}
