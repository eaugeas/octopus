package tree

func leftRotateNode(t *Tree, n *Node) {
	target := n.right
	if isSentinel(target) {
		return
	}

	n.right = target.left
	if isNotSentinel(target.left) {
		target.left.parent = n
	}
	target.parent = n.parent

	switch {
	case isSentinel(n.parent):
		t.root = target
	case n == n.parent.left:
		n.parent.left = target
	case n == n.parent.right:
		n.parent.right = target
	default:
		panic("unreachable statement")
	}

	target.left = n
	n.parent = target
}

func rightRotateNode(t *Tree, n *Node) {
	target := n.left
	if isSentinel(target) {
		return
	}

	n.left = target.right
	if isNotSentinel(target.right) {
		target.right.parent = n
	}

	target.parent = n.parent

	switch {
	case isSentinel(n.parent):
		t.root = target
	case n == n.parent.left:
		n.parent.left = target
	case n == n.parent.right:
		n.parent.right = target
	default:
		panic("unreachable statement")
	}

	target.right = n
	n.parent = target
}
