package hot

import (
	"bytes"
	"unsafe"
)

// tree is the Height Optimized Trie. The root is nil (empty), a *leaf
// (single key) or a compound node. The tree is not safe for concurrent use.
type tree struct {
	root    unsafe.Pointer
	size    int
	version int
}

func newTree() *tree {
	return &tree{}
}

// pathEntry records one step of a root-to-leaf descent: the compound node
// and the entry index chosen in it.
type pathEntry struct {
	n   unsafe.Pointer
	idx int
}

// Search retrieves the value associated with the specified key in the tree.
//
// The fast path descends with the raw key bytes: for keys without NUL bytes
// the escaped encoding is the identity, and for keys that do contain NUL the
// final leaf validation rejects any wrong candidate, in which case the
// search is retried once with the escaped encoding.
func (t *tree) Search(key Key) (Value, bool) {
	cur := t.root
	if cur == nil {
		return nil, false
	}
	if l := lookup(cur, key, key); l != nil {
		return l.value, true
	}
	if ek := escapeKey(key); len(ek) != len(key) {
		if l := lookup(t.root, ek, key); l != nil {
			return l.value, true
		}
	}
	return nil, false
}

// lookup descends to the result candidate for the encoded key and validates
// it against the original key (paper Listing 5).
func lookup(cur unsafe.Pointer, ek []byte, key Key) *leaf {
	for {
		switch hdr(cur).kind {
		case kindLeaf:
			l := (*leaf)(cur)
			if bytes.Equal(l.key, key) {
				return l
			}
			return nil
		case kindNode8:
			n := (*node8)(cur)
			cur = n.children[match8(&n.partialKeys, n.extract(ek), n.hdr.num)&(maxFanout-1)]
		case kindNode16:
			n := (*node16)(cur)
			cur = n.children[match16(&n.partialKeys, n.extract(ek), n.hdr.num)&(maxFanout-1)]
		default:
			n := (*node32)(cur)
			cur = n.children[match32(&n.partialKeys, n.extract(ek), n.hdr.num)&(maxFanout-1)]
		}
	}
}

// minimumLeaf returns the smallest leaf of a subtree.
func minimumLeaf(p unsafe.Pointer) *leaf {
	for !isLeaf(p) {
		p = childAt(p, 0)
	}
	return (*leaf)(p)
}

// maximumLeaf returns the largest leaf of a subtree.
func maximumLeaf(p unsafe.Pointer) *leaf {
	for !isLeaf(p) {
		p = childAt(p, int(hdr(p).num)-1)
	}
	return (*leaf)(p)
}

// Minimum retrieves the leaf with the smallest key in the tree.
func (t *tree) Minimum() (Value, bool) {
	if t.root == nil {
		return nil, false
	}
	return minimumLeaf(t.root).value, true
}

// Maximum retrieves the leaf with the largest key in the tree.
func (t *tree) Maximum() (Value, bool) {
	if t.root == nil {
		return nil, false
	}
	return maximumLeaf(t.root).value, true
}

// Size returns the number of key-value pairs stored in the tree.
func (t *tree) Size() int {
	return t.size
}

// descend walks from the root to the result candidate leaf for the encoded
// key, recording the path, and returns the candidate leaf.
func (t *tree) descend(ek []byte, path []pathEntry) (unsafe.Pointer, []pathEntry) {
	cur := t.root
	for {
		var idx int
		switch hdr(cur).kind {
		case kindLeaf:
			return cur, path
		case kindNode8:
			n := (*node8)(cur)
			idx = match8(&n.partialKeys, n.extract(ek), n.hdr.num) & (maxFanout - 1)
			path = append(path, pathEntry{cur, idx})
			cur = n.children[idx]
		case kindNode16:
			n := (*node16)(cur)
			idx = match16(&n.partialKeys, n.extract(ek), n.hdr.num) & (maxFanout - 1)
			path = append(path, pathEntry{cur, idx})
			cur = n.children[idx]
		default:
			n := (*node32)(cur)
			idx = match32(&n.partialKeys, n.extract(ek), n.hdr.num) & (maxFanout - 1)
			path = append(path, pathEntry{cur, idx})
			cur = n.children[idx]
		}
	}
}

// install replaces the node at path level i with p, keeping the path entry
// current for later height fix-ups.
func (t *tree) install(path []pathEntry, i int, p unsafe.Pointer) {
	path[i].n = p
	if i == 0 {
		t.root = p
	} else {
		setChildAt(path[i-1].n, path[i-1].idx, p)
	}
}
