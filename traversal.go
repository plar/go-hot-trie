package hot

import (
	"bytes"
	"unsafe"
)

// traverseOptions merges option flags, defaulting to TraverseLeaf.
func traverseOptions(options ...int) int {
	opts := 0
	for _, o := range options {
		opts |= o
	}
	if opts&TraverseAll == 0 {
		opts |= TraverseLeaf
	}
	return opts
}

// ForEach iterates over the tree in key order (pre-order for internal
// nodes), invoking the callback for every node selected by the options.
func (t *tree) ForEach(callback Callback, options ...int) {
	if t.root == nil {
		return
	}
	forEach(t.root, callback, traverseOptions(options...))
}

func forEach(p unsafe.Pointer, cb Callback, opts int) bool {
	if isLeaf(p) {
		if opts&TraverseLeaf != 0 {
			return cb((*leaf)(p))
		}
		return true
	}
	if opts&TraverseNode != 0 && !cb(asNode(p)) {
		return false
	}
	ch := childrenOf(p)
	reverse := opts&TraverseReverse != 0
	for i := range ch {
		idx := i
		if reverse {
			idx = len(ch) - 1 - i
		}
		if !forEach(ch[idx], cb, opts) {
			return false
		}
	}
	return true
}

// ForEachPrefix iterates over the leaves whose keys start with keyPrefix.
// Keys sharing a prefix are contiguous in key order: subtrees entirely
// before the range are pruned with an O(depth) boundary-leaf peek, and the
// traversal stops as soon as it walks past the range.
func (t *tree) ForEachPrefix(keyPrefix Key, callback Callback, options ...int) {
	if t.root == nil {
		return
	}
	opts := traverseOptions(options...)
	reverse := opts&TraverseReverse != 0
	forEachPrefix(t.root, keyPrefix, callback, reverse)
}

func forEachPrefix(p unsafe.Pointer, prefix Key, cb Callback, reverse bool) bool {
	if isLeaf(p) {
		l := (*leaf)(p)
		if bytes.HasPrefix(l.key, prefix) {
			return cb(l)
		}
		// Continue while still before the matching range, stop once past it.
		if reverse {
			return bytes.Compare(l.key, prefix) > 0
		}
		return bytes.Compare(l.key, prefix) < 0
	}
	ch := childrenOf(p)
	for i := range ch {
		idx := i
		if reverse {
			idx = len(ch) - 1 - i
		}
		c := ch[idx]
		// Prune subtrees that lie entirely before the matching range (any
		// key with the prefix compares >= the prefix itself); the leaf-level
		// "stop once past" rule above prunes the tail.
		if !reverse {
			if bytes.Compare(maximumLeaf(c).key, prefix) < 0 {
				continue
			}
		} else {
			if mk := minimumLeaf(c).key; !bytes.HasPrefix(mk, prefix) && bytes.Compare(mk, prefix) > 0 {
				continue
			}
		}
		if !forEachPrefix(c, prefix, cb, reverse) {
			return false
		}
	}
	return true
}
