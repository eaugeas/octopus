package tree

type color uint

const (
	red   color = 0
	black color = 1
)

type redBlack struct {
	unbalanced
}

func isRed(n *Node) bool {
	return color(n.metadata&0x00000001) == red
}

func isBlack(n *Node) bool {
	return color(n.metadata&0x00000001) == black
}

func copyColor(dest *Node, source *Node) {
	dest.metadata = (source.metadata & 0xfffffffe) | (source.metadata & 0x00000001)
}

func setRed(n *Node) {
	n.metadata = n.metadata & 0xfffffffe
}

func setBlack(n *Node) {
	n.metadata = n.metadata | 0x00000001
}

func (rb redBlack) Insert(t *Tree, n *Node) {
	rb.unbalanced.Insert(t, n)
	setBlack(n.left)
	setBlack(n.right)
	rb.fixInsert(t, n)
}

func (rb redBlack) Delete(t *Tree, n *Node) {
	var target *Node
	wasNodeBlack := isBlack(n)

	switch {
	case isSentinel(n.left):
		target = n.right
		rb.unbalanced.Transplant(t, n, target)
	case isSentinel(n.right):
		target = n.left
		rb.unbalanced.Transplant(t, n, target)
	default:
		min := n.right.Min()
		wasNodeBlack = isBlack(min)
		target = min.right

		if min.parent == n {
			target.parent = min
		} else {
			rb.unbalanced.Transplant(t, min, min.right)
			min.right = n.right
			min.right.parent = min
		}

		rb.unbalanced.Transplant(t, n, min)
		min.left = n.left
		min.left.parent = min
		copyColor(min, n)
	}

	if wasNodeBlack {
		rb.fixDelete(t, target)
	}
}

func (rb redBlack) fixDelete(t *Tree, n *Node) {
	for n != t.root && isBlack(n) {
		if n == n.parent.left {
			target := n.parent.right
			if isRed(target) {
				setBlack(target)
				setRed(n.parent)
				leftRotateNode(t, n.parent)
				target = n.parent.right
			}

			switch {
			case isBlack(target.left) && isBlack(target.right):
				setRed(target)
				n = n.parent
			case isBlack(target.right):
				setBlack(target.left)
				setRed(target)
				rightRotateNode(t, target)
				target = n.parent.right
			default:
				copyColor(target, n.parent)
				setBlack(n.parent)
				setBlack(target.right)
				leftRotateNode(t, n.parent)
				n = t.root
			}

		} else {
			target := n.parent.left
			if isRed(target) {
				setBlack(target)
				setRed(n.parent)
				rightRotateNode(t, n.parent)
				target = n.parent.left
			}

			switch {
			case isBlack(target.left) && isBlack(target.right):
				setRed(target)
				n = n.parent
			case isBlack(target.left):
				setBlack(target.right)
				setRed(target)
				leftRotateNode(t, target)
				target = n.parent.left
			default:
				copyColor(target, n.parent)
				setBlack(n.parent)
				setBlack(target.left)
				rightRotateNode(t, n.parent)
				n = t.root
			}
		}
	}

	// ensure that if node is the root of the tree it will
	// remain black after a call to deleteFixup
	setBlack(n)
}

func (rb redBlack) fixInsert(t *Tree, n *Node) {
	setRed(n)

	for isNotSentinel(n.parent) && isNotSentinel(n.parent.parent) && isRed(n.parent) {
		if n.parent == n.parent.parent.left {
			target := n.parent.parent.right

			switch {
			case isRed(target):
				setBlack(n.parent)
				setBlack(target)
				setRed(n.parent.parent)
				n = n.parent.parent
			case n == n.parent.right:
				n = n.parent
				leftRotateNode(t, n)
			case n == n.parent.left:
				setBlack(n.parent)
				setRed(n.parent.parent)
				rightRotateNode(t, n.parent.parent)
			default:
				panic("unreachable statement")
			}
		} else {
			target := n.parent.parent.left

			switch {
			case isRed(target):
				setBlack(n.parent)
				setBlack(target)
				setRed(n.parent.parent)
				n = n.parent.parent
			case n == n.parent.left:
				n = n.parent
				rightRotateNode(t, n)
			case n == n.parent.right:
				setBlack(n.parent)
				setRed(n.parent.parent)
				leftRotateNode(t, n.parent.parent)
			default:
				panic("unreachable statement")
			}
		}
	}

	setBlack(t.root)
}

// New creates a new instance of a tree using RedBlack
// as the modifier algorithm. The branches of the tree
// are balanced using the red black node algorithm.
func NewRedBlackTree(cmp Lesser) *Tree {
	n := newSentinelNode()
	setBlack(n)
	return &Tree{root: n, cmp: cmp, mod: redBlack{}}
}
