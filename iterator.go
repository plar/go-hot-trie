package hot

import "unsafe"

type iterFrame struct {
	p       unsafe.Pointer
	visited bool
	next    int
}

type iterator struct {
	t        *tree
	version  int
	opts     int
	stack    []iterFrame
	prepared Node
}

// Iterator returns an iterator over the tree in key order (pre-order for
// internal nodes when TraverseNode/TraverseAll is passed).
func (t *tree) Iterator(options ...int) Iterator {
	it := &iterator{t: t, version: t.version, opts: traverseOptions(options...)}
	if t.root != nil {
		it.stack = append(it.stack, iterFrame{p: t.root})
	}
	return it
}

// advance walks the stack to the next node matching the traversal options.
// Leaves are yielded directly at child expansion and never pushed; the only
// leaf frame possible is a leaf root.
func (it *iterator) advance() Node {
	for len(it.stack) > 0 {
		top := len(it.stack) - 1
		f := it.stack[top]
		if isLeaf(f.p) {
			it.stack = it.stack[:top]
			if it.opts&TraverseLeaf != 0 {
				return (*leaf)(f.p)
			}
			continue
		}
		if !f.visited {
			it.stack[top].visited = true
			if it.opts&TraverseNode != 0 {
				return asNode(f.p)
			}
			continue
		}
		ch := childrenOf(f.p)
		if f.next >= len(ch) {
			it.stack = it.stack[:top]
			continue
		}
		it.stack[top].next++
		idx := f.next
		if it.opts&TraverseReverse != 0 {
			idx = len(ch) - 1 - f.next
		}
		c := ch[idx]
		if isLeaf(c) {
			if it.opts&TraverseLeaf != 0 {
				return (*leaf)(c)
			}
			continue
		}
		it.stack = append(it.stack, iterFrame{p: c})
	}
	return nil
}

// HasNext returns true if there are more nodes to visit. It is idempotent.
func (it *iterator) HasNext() bool {
	if it.prepared != nil {
		return true
	}
	it.prepared = it.advance()
	return it.prepared != nil
}

// Next returns the next node, ErrNoMoreNodes when exhausted, or
// ErrConcurrentModification if the tree changed since the iterator was
// created.
func (it *iterator) Next() (Node, error) {
	if it.version != it.t.version {
		return nil, ErrConcurrentModification
	}
	if n := it.prepared; n != nil {
		it.prepared = nil
		return n, nil
	}
	if n := it.advance(); n != nil {
		return n, nil
	}
	return nil, ErrNoMoreNodes
}
