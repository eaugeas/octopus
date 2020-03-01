package tree

// unbalanced is a pair of algorithms that insert
// and delete nodes from the tree without applying any
// balancing strategy
type unbalanced struct{}

// Insert the node into the tree by preserving the Binary Search Tree
// properties but without applying any balancing algorithm
func (unbalanced) Insert(t *Tree, n *Node) {
	var parent *Node
	var isLeft bool

	curr := t.root

	for isNotSentinel(curr) {
		parent = curr
		if t.cmp.Less(n.Value, curr.Value) < 0 {
			isLeft = true
			curr = curr.left
		} else {
			isLeft = false
			curr = curr.right
		}
	}

	n.parent = parent

	switch {
	case parent == nil:
		t.root = n
		n.parent = newSentinelNode()
	case isLeft:
		parent.left = n
	default:
		parent.right = n
	}

	n.left = newSentinelNode()
	n.left.parent = n
	n.right = newSentinelNode()
	n.right.parent = n
}

// Delete the node from the tree by preserving the Binary Search Tree
// properties but without applying any balancing algorithm
func (m unbalanced) Delete(t *Tree, n *Node) {
	switch {
	case isSentinel(n.left):
		m.Transplant(t, n, n.right)
	case isSentinel(n.right):
		m.Transplant(t, n, n.left)
	default:
		min := n.Right().Min()
		if min.parent != n {
			m.Transplant(t, min, min.right)
			min.right = n.right
			min.right.parent = min
		}

		m.Transplant(t, n, min)
		min.left = n.left
		min.left.parent = min
	}
}

// transplant replaces one subtree as a child of its parent
// with another subtree
func (unbalanced) Transplant(t *Tree, u *Node, v *Node) {
	switch {
	case isSentinel(u.parent):
		t.root = v
	case u == u.parent.left:
		u.parent.left = v
	default:
		u.parent.right = v
	}

	v.parent = u.parent
}

// New creates a new instance of a tree using Unbalanced
// as the modifier algorithm. How balanced the branches
// of the tree are depends exclusively on the order
// of the insert and delete operations performed
// on the tree
func NewUnbalancedTree(cmp Lesser) *Tree {
	return &Tree{root: newSentinelNode(), cmp: cmp, mod: unbalanced{}}
}
