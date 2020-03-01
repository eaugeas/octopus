package tree

const sentinelValue = 0xa0000000

func isNotSentinel(n *Node) bool {
	return n.metadata&sentinelValue != sentinelValue
}

func isSentinel(n *Node) bool {
	return n.metadata&sentinelValue == sentinelValue
}

func newSentinelNode() *Node {
	return &Node{metadata: sentinelValue}
}

func nilIfSentinel(n *Node) *Node {
	if isSentinel(n) {
		return nil
	} else {
		return n
	}
}

// Node of a tree
type Node struct {
	Value interface{}

	metadata uint
	cmp      Lesser
	left     *Node
	right    *Node
	parent   *Node
}

// Left returns the node's left child
func (n *Node) Left() *Node {
	return nilIfSentinel(n.left)
}

// Right returns the node's right child
func (n *Node) Right() *Node {
	return nilIfSentinel(n.right)
}

// Parent returns the node's parent
func (n *Node) Parent() *Node {
	return nilIfSentinel(n.parent)
}

// Min returns the node in the subtree of the
// lowest order. It returns null if the tree
// is empty
func (n *Node) Min() *Node {
	curr := n

	for isNotSentinel(curr) && isNotSentinel(curr.left) {
		curr = curr.left
	}

	return nilIfSentinel(curr)
}

// Max returns the node in the subtree of the
// highest order. It returns null if tree
// is empty
func (n *Node) Max() *Node {
	curr := n

	for isNotSentinel(curr) && isNotSentinel(curr.right) {
		curr = curr.right
	}

	return nilIfSentinel(curr)
}

// Contains returns true if the subtree contains at
// least one node with value v
func (n *Node) Contains(v interface{}) bool {
	return n.Find(v) != nil
}

// Find returns the first node in the tree that
// contains a value equal to the one provided
func (n *Node) Find(v interface{}) (res *Node) {
	for curr := n; isNotSentinel(curr); {
		if curr.cmp.Less(v, curr.Value) < 0 {
			curr = curr.left
		} else {
			if curr.Value == v {
				res = curr
				break
			}

			curr = curr.right
		}
	}

	if res == nil {
		return nil
	}

	return nilIfSentinel(res)
}

// Count returns the number of occurrences of v
// in the current subtree
func (n *Node) Count(v interface{}) (count int) {
	for curr := n; isNotSentinel(curr); {
		if curr.cmp.Less(curr.Value, v) < 0 {
			curr = curr.left
		} else {
			if curr.Value == v {
				// if we find an equality, keep checking for
				// values until we find one that is not equal or
				// we reach a leaf
				count++
				curr = curr.right
				for isNotSentinel(curr) && curr.Value == v {
					count++
					curr = curr.right
				}

				break
			}

			curr = curr.right
		}
	}

	return count
}

// Successor finds the successor of the current
// node in its tree. That is, the node in the tree
// of the lowest order that is strictly greater than
// the current node.
func (n *Node) Successor() *Node {
	if isNotSentinel(n.right) {
		return n.right.Min()
	}

	curr := n
	prev := n.parent
	for isNotSentinel(curr) && prev == curr.right {
		prev = curr
		curr = curr.parent
	}

	return curr
}

// Higher returns the node in the tree that has the
// smallest order which is higher than Value
func (n *Node) Higher(v interface{}) *Node {
	var higher *Node
	curr := n

	for isNotSentinel(curr) {
		if n.cmp.Less(v, curr.Value) <= 0 {
			higher = curr
			curr = curr.left
		} else {
			curr = curr.right
		}
	}

	return higher
}

// Lower returns the node in the tree that has the
// higher order which is lower than Value
func (n *Node) Lower(v interface{}) *Node {
	var lower *Node
	curr := n

	for isNotSentinel(curr) {
		if n.cmp.Less(v, curr.Value) < 0 {
			curr = curr.left
		} else {
			lower = curr
			curr = curr.right
		}
	}

	return lower
}

// Predecessor finds the predecessor of the current
// node in its tree. That is, the node in the tree
// of the highest order that is strictly smaller than
// the current node.
func (n *Node) Predecessor() *Node {
	if isNotSentinel(n.left) {
		return n.left.Max()
	}

	curr := n
	prev := n.parent
	for isNotSentinel(curr) && prev == curr.left {
		prev = curr
		curr = curr.parent
	}

	return curr
}

// InOrderWalk implements an in order walk
// on the subtree using Morris traversal.
func (n *Node) InOrderWalk(fn func(*Node)) {
	var prev *Node

	for curr := n; curr != nil; {
		if curr.left == nil {
			if isNotSentinel(curr) {
				fn(curr)
			}

			curr = curr.right

		} else {
			prev = curr.left
			for prev.right != nil && prev.right != curr {
				prev = prev.right
			}

			if prev.right == nil {
				// update prev.right to the curr node so that after
				// visiting the left subtree there's a reference to the
				// right subtree that has been ignored
				prev.right = curr
				curr = curr.left
			} else {
				// restore the value of prev.right
				prev.right = nil
				if isNotSentinel(curr) {
					fn(curr)
				}
				curr = curr.right
			}
		}
	}
}

// PreOrderWalk implements a pre order walk
// on the subtree using Morris traversal.
func (n *Node) PreOrderWalk(fn func(*Node)) {
	var prev *Node

	for curr := n; curr != nil; {
		if curr.left == nil {
			if isNotSentinel(curr) {
				fn(curr)
			}
			curr = curr.right

		} else {
			prev = curr.left
			for prev.right != nil && prev.right != curr {
				prev = prev.right
			}

			if prev.right == nil {
				// update prev.right to the curr node so that after
				// visiting the left subtree there's a reference to the
				// right subtree that has been ignored
				prev.right = curr
				if isNotSentinel(curr) {
					fn(curr)
				}

				curr = curr.left

			} else {
				// restore the value of prev.right
				prev.right = nil
				curr = curr.right
			}
		}
	}
}

// PostOrderWalk implements a post order walk
// on the subtree using Morris traversal.
func (n *Node) PostOrderWalk(fn func(*Node)) {
	var prev *Node

	for curr := n; curr != nil; {
		if curr.right == nil {
			if isNotSentinel(curr) {
				fn(curr)
			}

			curr = curr.left

		} else {
			prev = curr.right
			for prev.left != nil && prev.left != curr {
				prev = prev.left
			}

			if prev.left == nil {
				// update prev.left to the curr node so that after
				// visiting the right subtree there's a reference to the
				// left subtree that has been ignored
				prev.left = curr
				curr = curr.right
			} else {
				// restore the value of prev.left
				prev.left = nil
				if isNotSentinel(curr) {
					fn(curr)
				}

				curr = curr.left
			}
		}
	}
}

// Tree represents a binary search tree
type Tree struct {
	root *Node
	cmp  Lesser
	mod  modifier
	len  int
}

// Len returns the number of nodes in the tree
func (t *Tree) Len() int {
	return t.len
}

// Empty returns true if the tree has no nodes
func (t *Tree) Empty() bool {
	return isSentinel(t.root)
}

// Root returns the root of the tree. It returns
// nil for an empty tree
func (t *Tree) Root() *Node {
	return nilIfSentinel(t.root)
}

// Min returns the node in the tree with the
// lowest value. It returns null if the tree
// is empty
func (t *Tree) Min() *Node {
	return t.root.Min()
}

// Max returns the node in the tree with the
// highest value. It returns null if tree
// is empty
func (t *Tree) Max() *Node {
	return t.root.Max()
}

// Higher returns the node in the tree that has the
// smallest order which is higher than Value
func (t *Tree) Higher(v interface{}) *Node {
	return t.root.Higher(v)
}

// Lower returns the node in the tree that has the
// higher order which is lower than Value
func (t *Tree) Lower(v interface{}) *Node {
	return t.root.Lower(v)
}

// Contains returns true if the tree contains at
// least one node with value v
func (t *Tree) Contains(v interface{}) bool {
	return t.root.Find(v) != nil
}

// Count returns the number of occurrences of v
// in the tree
func (t *Tree) Count(v interface{}) int {
	return t.root.Count(v)
}

// Find returns the first node in the tree that
// contains a value equal to the one provided
func (t *Tree) Find(v interface{}) *Node {
	return t.root.Find(v)
}

// InOrderWalk implements an in order walk
// on the tree using Morris traversal.
func (t *Tree) InOrderWalk(fn func(*Node)) {
	t.root.InOrderWalk(fn)
}

// PreOrderWalk implements a pre order walk
// on the tree using Morris traversal.
func (t *Tree) PreOrderWalk(fn func(*Node)) {
	t.root.PreOrderWalk(fn)
}

// PostOrderWalk implements a post order walk
// on the tree using Morris traversal.
func (t *Tree) PostOrderWalk(fn func(*Node)) {
	t.root.PostOrderWalk(fn)
}

// Insert a value into the tree
func (t *Tree) Insert(v interface{}) {
	t.mod.Insert(t, &Node{
		Value:    v,
		cmp:      t.cmp,
		left:     nil,
		right:    nil,
		metadata: 0,
	})
	t.len++
}

// Delete the first node on the tree that has value
// equal to v
func (t *Tree) Delete(v interface{}) bool {
	n := t.Find(v)
	if n == nil {
		return false
	}

	t.mod.Delete(t, n)
	t.len--
	return true
}
