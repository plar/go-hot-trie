// Package hot implements HOT, the Height Optimized Trie (Binna et al.,
// SIGMOD 2018 / ACM TODS 2022), an in-memory ordered index for byte-string
// keys. The public interface mirrors github.com/plar/go-adaptive-radix-tree.
//
// HOT combines the nodes of a binary Patricia trie into compound nodes with a
// data-dependent span and a fixed maximum fanout k=32, keeping the overall
// tree height minimal. Compound nodes are searched with data-parallel SIMD
// comparisons over sparse partial keys (built with GOEXPERIMENT=simd on
// amd64; a scalar fallback is used otherwise).
package hot

import "errors"

// Node types.
const (
	// Leaf is a leaf node storing a key/value pair.
	Leaf Kind = 0

	// Node8 is a compound node whose sparse partial keys fit in 8 bits.
	Node8 Kind = 1

	// Node16 is a compound node whose sparse partial keys fit in 16 bits.
	Node16 Kind = 2

	// Node32 is a compound node whose sparse partial keys fit in 32 bits.
	Node32 Kind = 3
)

// Traverse Options.
const (
	// Iterate only over leaf nodes.
	TraverseLeaf = 1

	// Iterate only over non-leaf nodes.
	TraverseNode = 2

	// Iterate over all nodes in the tree.
	TraverseAll = TraverseLeaf | TraverseNode

	// Iterate in reverse order.
	TraverseReverse = 4
)

// These errors can be returned when iteration over the tree.
var (
	ErrConcurrentModification = errors.New("concurrent modification has been detected")
	ErrNoMoreNodes            = errors.New("there are no more nodes in the tree")
)

// Kind is a node type.
type Kind int

// String returns string representation of the Kind value.
func (k Kind) String() string {
	return []string{"Leaf", "Node8", "Node16", "Node32"}[k]
}

// Key represents the type used for keys in the Height Optimized Trie.
// It can consist of any byte sequence, including Unicode characters and null bytes.
type Key []byte

// Value is an interface representing the value type stored in the tree.
// Any type of data can be stored as a Value.
type Value any

// Callback defines the function type used during tree traversal.
// It is invoked for each node visited in the traversal.
// If the callback function returns false, the iteration is terminated early.
type Callback func(node Node) (cont bool)

// Node represents a node within the Height Optimized Trie.
type Node interface {
	// Kind returns the type of the node, distinguishing between leaf and internal nodes.
	Kind() Kind

	// Key returns the key associated with a leaf node.
	// This method should only be called on leaf nodes.
	// Calling this on a non-leaf node will return nil.
	Key() Key

	// Value returns the value stored in a leaf node.
	// This method should only be called on leaf nodes.
	// Calling this on a non-leaf node will return nil.
	Value() Value
}

// Iterator provides a mechanism to traverse nodes in key order within the tree.
type Iterator interface {
	// HasNext returns true if there are more nodes to visit during the iteration.
	// Use this method to check for remaining nodes before calling Next.
	HasNext() bool

	// Next returns the next node in the iteration and advances the iterator's position.
	// If the iteration has no more nodes, it returns ErrNoMoreNodes error.
	// Ensure you call HasNext before invoking Next to avoid errors.
	// If the tree has been structurally modified since the iterator was created,
	// it returns an ErrConcurrentModification error.
	Next() (Node, error)
}

// Tree is a Height Optimized Trie interface.
type Tree interface {
	// Insert adds a new key-value pair into the tree.
	// If the key already exists in the tree, it updates its value and returns the old value along with true.
	// If the key is new, it returns nil and false.
	//
	// The tree stores the key slice without copying (as go-adaptive-radix-tree
	// does): the caller must not modify the key bytes after insertion.
	Insert(key Key, value Value) (oldValue Value, updated bool)

	// Delete removes the specified key and its associated value from the tree.
	// If the key is found and deleted, it returns the removed value and true.
	// If the key does not exist, it returns nil and false.
	Delete(key Key) (value Value, deleted bool)

	// Search retrieves the value associated with the specified key in the tree.
	// If the key exists, it returns the value and true.
	// If the key does not exist, it returns nil and false.
	Search(key Key) (value Value, found bool)

	// ForEach iterates over all the nodes in the tree, invoking a provided callback function for each node.
	// By default, it processes leaf nodes in ascending order.
	// The iteration can be customized using options:
	// - Pass TraverseReverse to iterate over nodes in descending order.
	// The iteration stops if the callback function returns false, allowing for early termination.
	ForEach(callback Callback, options ...int)

	// ForEachPrefix iterates over all leaf nodes whose keys start with the specified keyPrefix,
	// invoking a provided callback function for each matching node.
	// By default, the iteration processes nodes in ascending order.
	// Use the TraverseReverse option to iterate over nodes in descending order.
	// Iteration stops if the callback function returns false, allowing for early termination.
	ForEachPrefix(keyPrefix Key, callback Callback, options ...int)

	// Iterator returns an iterator for traversing leaf nodes in the tree.
	// By default, the iteration occurs in ascending order.
	// To traverse nodes in reverse (descending) order, pass the TraverseReverse option.
	Iterator(options ...int) Iterator

	// Minimum retrieves the leaf node with the smallest key in the tree.
	// If such a leaf is found, it returns its value and true.
	// If the tree is empty, it returns nil and false.
	Minimum() (Value, bool)

	// Maximum retrieves the leaf node with the largest key in the tree.
	// If such a leaf is found, it returns its value and true.
	// If the tree is empty, it returns nil and false.
	Maximum() (Value, bool)

	// Size returns the number of key-value pairs stored in the tree.
	Size() int
}

// New creates a new Height Optimized Trie.
func New() Tree {
	return newTree()
}
