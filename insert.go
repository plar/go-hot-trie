package hot

import (
	"bytes"
	"math/bits"
	"unsafe"
)

// Insert adds a new key-value pair into the tree, implementing the four
// cases of paper §3.2.1 (Listing 1): normal insert, leaf-node pushdown, and
// overflow handling by parent pull-up or intermediate node creation.
func (t *tree) Insert(key Key, value Value) (Value, bool) {
	ek := escapeKey(key)
	if t.root == nil {
		t.root = newLeaf(key, value, len(ek) != len(key))
		t.size++
		t.version++
		return nil, false
	}
	var pathArr [16]pathEntry
	cand, path := t.descend(ek, pathArr[:0])

	l := (*leaf)(cand)
	if bytes.Equal(l.key, key) {
		old := l.value
		l.value = value
		t.version++
		return old, true
	}

	mbit := uint32(mismatchBit(ek, l.ekey()))
	newBit := keyBitAt(ek, mbit)
	nl := newLeaf(key, value, len(ek) != len(key))

	// Locate the node containing the mismatching BiNode: the first node on
	// the path with an on-path BiNode position exceeding mbit. BiNode
	// positions are strictly increasing along the path, so first find the
	// last node k whose internal root position (an O(1) mask lookup) is
	// still <= mbit; only that node needs its deepest on-path position (the
	// parent BiNode of the chosen entry) computed.
	aff := -1
	k := -1
	for i := range path {
		if minPos(path[i].n) > mbit {
			break
		}
		k = i
	}
	switch {
	case k == -1:
		// mbit precedes the root node's internal root BiNode.
		if len(path) > 0 {
			aff = 0
		}
	case deepestOnPathPos(path[k].n, path[k].idx) > mbit:
		aff = k
	case k+1 < len(path):
		// The mismatching BiNode is the internal root of the next node.
		aff = k + 1
	}

	if aff == -1 {
		// The mismatching entity is the result candidate leaf entry itself.
		if len(path) == 0 {
			// The root is a leaf: grow a first compound node.
			t.root = makePair(mbit, newBit, unsafe.Pointer(l), nl)
		} else if last := len(path) - 1; hdr(path[last].n).height > 1 {
			// Leaf-node pushdown: replace the hoisted leaf entry with a new
			// height-1 node holding the old leaf and the new key.
			setChildAt(path[last].n, path[last].idx, makePair(mbit, newBit, unsafe.Pointer(l), nl))
		} else {
			t.insertAt(path, last, mbit, newBit, nl)
		}
	} else {
		t.insertAt(path, aff, mbit, newBit, nl)
	}

	t.size++
	t.version++
	return nil, false
}

// makeNodePair builds a 2-entry node discriminated at position q (children
// already in key order).
func makeNodePair(q uint32, l, r unsafe.Pointer) unsafe.Pointer {
	var b nb
	b.np = 1
	b.positions[0] = q
	b.ne = 2
	b.sparse[1] = rankBit(0)
	b.children[0] = l
	b.children[1] = r
	return b.materialize()
}

// makePair builds a 2-entry node with existing going left/right of the new
// child depending on the new key's bit at q.
func makePair(q, newBit uint32, existing, newChild unsafe.Pointer) unsafe.Pointer {
	if newBit == 1 {
		return makeNodePair(q, existing, newChild)
	}
	return makeNodePair(q, newChild, existing)
}

// minPos returns the position of a node's internal root BiNode (rank 0, the
// smallest discriminative position) in O(1).
func minPos(p unsafe.Pointer) uint32 {
	switch hdr(p).kind {
	case kindNode8:
		return (*node8)(p).minPos()
	case kindNode16:
		return (*node16)(p).minPos()
	default:
		return (*node32)(p).minPos()
	}
}

func (n *node[W]) minPos() uint32 {
	if n.spec == specSingle {
		return n.offset*8 + uint32(bits.LeadingZeros64(n.mask))
	}
	return n.byteOffsets[0]*8 + uint32(bits.LeadingZeros8(n.byteMasks[0]))
}

// rankPos returns the discriminative position with the given rank.
func (n *node[W]) rankPos(r int) uint32 {
	if n.spec == specSingle {
		m := n.mask
		for ; r > 0; r-- {
			m &^= 1 << (63 - bits.LeadingZeros64(m))
		}
		return n.offset*8 + uint32(bits.LeadingZeros64(m))
	}
	// Walk the per-byte masks to the byte holding rank r, then to the bit.
	for j := 0; ; j++ {
		m := n.byteMasks[j]
		if c := bits.OnesCount8(m); r >= c {
			r -= c
			continue
		}
		for ; r > 0; r-- {
			m &^= 1 << (7 - bits.LeadingZeros8(m))
		}
		return n.byteOffsets[j]*8 + uint32(bits.LeadingZeros8(m))
	}
}

// deepestOnPathPos returns the position of the parent BiNode of entry c: the
// deepest BiNode on the search path inside this node.
func deepestOnPathPos(p unsafe.Pointer, c int) uint32 {
	switch hdr(p).kind {
	case kindNode8:
		return dopp((*node8)(p), c)
	case kindNode16:
		return dopp((*node16)(p), c)
	default:
		return dopp((*node32)(p), c)
	}
}

func dopp[W pkWidth](n *node[W], c int) uint32 {
	dl, dr := neighborDiffRanks(n.partialKeys[:n.hdr.num], c)
	return n.rankPos(max(dl, dr))
}

// insertAt performs a normal insert of the new discriminating BiNode into
// the node at path level i, handling overflow. The common case (no overflow,
// no partial key width upgrade) mutates the node in place; structural cases
// go through the builder.
func (t *tree) insertAt(path []pathEntry, i int, mbit uint32, newBit uint32, child unsafe.Pointer) {
	p := path[i].n
	ok := false
	switch hdr(p).kind {
	case kindNode8:
		ok = ipInsert((*node8)(p), path[i].idx, mbit, newBit, child)
	case kindNode16:
		ok = ipInsert((*node16)(p), path[i].idx, mbit, newBit, child)
	default:
		ok = ipInsert((*node32)(p), path[i].idx, mbit, newBit, child)
	}
	if ok {
		return
	}
	var b nb
	load(p, &b)
	b.insertEntry(mbit, newBit, path[i].idx, child)
	t.finishInsert(path, i, &b, hdr(p).height)
}

// finishInsert materializes the modified builder at path level i. On
// overflow the node is split at its root BiNode; per paper §3.2.1 the split
// pair is pulled up into the parent when h(n)+1 == h(parent) (growing the
// tree upwards, possibly recursing and eventually creating a new root),
// otherwise an intermediate node is created.
func (t *tree) finishInsert(path []pathEntry, i int, b *nb, origHeight uint8) {
	if b.ne <= maxFanout {
		t.install(path, i, b.materialize())
		return
	}
	l, r, q := b.split()
	if i == 0 {
		t.root = makeNodePair(q, l, r)
		return
	}
	parent := path[i-1].n
	if origHeight+1 == hdr(parent).height {
		var pb nb
		load(parent, &pb)
		pb.replaceWithPair(path[i-1].idx, q, l, r)
		t.finishInsert(path, i-1, &pb, hdr(parent).height)
	} else {
		t.install(path, i, makeNodePair(q, l, r))
	}
}
