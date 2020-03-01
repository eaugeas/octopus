package tree

// modifier is a pair of algorithms used to insert
// and remove nodes from the tree
type modifier interface {
	// Insert a node into the tree
	Insert(t *Tree, n *Node)

	// Delete a node from the tree
	Delete(t *Tree, n *Node)
}
