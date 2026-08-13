package hot

import (
	"bytes"
	"unsafe"
)

// Delete removes the specified key from the tree, implementing the three
// cases of paper §3.2.2 (Listing 2): normal deletion (with the 2-entry node
// collapsing to its remaining entry, the inverse of leaf pushdown), and
// underflow resolution by simple BiNode pull-down or node merge, both of
// which may recurse up the tree.
//
// Like Search, the descent uses the raw key bytes and only retries with the
// escaped encoding when a key containing NUL bytes is not found directly.
func (t *tree) Delete(key Key) (Value, bool) {
	if t.root == nil {
		return nil, false
	}
	var pathArr [16]pathEntry
	cand, path := t.descend(key, pathArr[:0])
	if !bytes.Equal((*leaf)(cand).key, key) {
		ek := escapeKey(key)
		if len(ek) == len(key) {
			return nil, false
		}
		cand, path = t.descend(ek, pathArr[:0])
		if !bytes.Equal((*leaf)(cand).key, key) {
			return nil, false
		}
	}
	return t.deleteAt(cand, path)
}

func (t *tree) deleteAt(cand unsafe.Pointer, path []pathEntry) (Value, bool) {
	val := (*leaf)(cand).value
	t.size--
	t.version++
	if len(path) == 0 {
		t.root = nil
		return val, true
	}
	i := len(path) - 1
	p := path[i].n
	if hdr(p).num > 2 {
		// Normal deletion, in place. Removing a leaf entry never changes the
		// node's height; underflow resolution fixes ancestor heights itself.
		ipRemoveAt(p, path[i].idx)
		t.resolveUnderflow(path, i)
		return val, true
	}
	// Inverse leaf pushdown: hoist the remaining entry.
	t.install(path, i, childAt(p, 1-path[i].idx))
	t.fixHeightsFrom(path, i-1)
	return val, true
}

// underflowProbe locates the parent BiNode of entry j and its sibling
// subtree, reporting the sibling entry index, its side, whether the sibling
// subtree is a single entry, and the parent BiNode's rank.
func underflowProbe(p unsafe.Pointer, j int) (sIdx int, sLeft, single bool, rq int) {
	switch hdr(p).kind {
	case kindNode8:
		return probe((*node8)(p), j)
	case kindNode16:
		return probe((*node16)(p), j)
	default:
		return probe((*node32)(p), j)
	}
}

func probe[W pkWidth](n *node[W], j int) (sIdx int, sLeft, single bool, rq int) {
	num := int(n.hdr.num)
	wb := wBits[W]()
	dl, dr := neighborDiffRanks(n.partialKeys[:num], j)
	if dr > dl {
		rq, sIdx, sLeft = dr, j+1, false
	} else {
		rq, sIdx, sLeft = dl, j-1, true
	}
	pm := ^W(0) << (wb - uint(rq))
	pfx := n.partialKeys[j] & pm
	if sLeft {
		single = sIdx == 0 || n.partialKeys[sIdx-1]&pm != pfx
	} else {
		single = sIdx+1 == num || n.partialKeys[sIdx+1]&pm != pfx
	}
	return
}

// rankPosOf returns the discriminative position with the given rank.
func rankPosOf(p unsafe.Pointer, r int) uint32 {
	switch hdr(p).kind {
	case kindNode8:
		return (*node8)(p).rankPos(r)
	case kindNode16:
		return (*node16)(p).rankPos(r)
	default:
		return (*node32)(p).rankPos(r)
	}
}

// resolveUnderflow checks the node currently installed at path level i
// against its sibling and applies node merge or simple BiNode pull-down,
// walking up the tree while entries keep disappearing from parents, then
// refreshes ancestor heights if anything changed.
func (t *tree) resolveUnderflow(path []pathEntry, i int) {
	changed := false
	for i > 0 {
		parent := path[i-1].n
		j := path[i-1].idx
		// n is always a compound node here: the first iteration re-reads the
		// node deleteAt just modified, later ones the merged replacement.
		n := childAt(parent, j)

		// An underflow cannot occur when the sibling is a BiNode, i.e. when
		// the sibling subtree holds more than one entry.
		sIdx, sLeft, single, rq := underflowProbe(parent, j)
		if !single {
			break
		}

		s := childAt(parent, sIdx)
		cn, cs := int(hdr(n).num), 1
		if !isLeaf(s) {
			cs = int(hdr(s).num)
		}
		if cn+cs > maxFanout {
			break
		}
		hn, hs := hdr(n).height, entryHeight(s)
		if hn < hs {
			break
		}

		q := rankPosOf(parent, rq)
		var combined unsafe.Pointer
		if hs == hn {
			if sLeft {
				combined = mergeNodes(s, n, q)
			} else {
				combined = mergeNodes(n, s, q)
			}
		} else {
			combined = absorb(n, q, s, sLeft)
		}
		changed = true
		path[i].n = combined
		if hdr(parent).num > 2 {
			// Remove the absorbed sibling entry from the parent in place.
			setChildAt(parent, j, combined)
			ipRemoveAt(parent, sIdx)
		} else {
			// The parent held only the two merged entries: hoist combined.
			t.install(path, i-1, combined)
		}
		i--
	}
	if changed {
		t.fixHeightsFrom(path, i)
	}
}

// fixHeightsFrom recomputes stored heights bottom-up along the path,
// starting at level from. The walk deliberately runs all the way to the
// root: a hoist replaces a level with a node whose stored height is already
// consistent, so an early "unchanged here" break could skip a stale
// ancestor above the replacement. Structural deletes are rare enough that
// the full walk is cheap.
func (t *tree) fixHeightsFrom(path []pathEntry, from int) {
	for l := min(from, len(path)-1); l >= 0; l-- {
		n := path[l].n
		if h := computeHeight(n); hdr(n).height != h {
			hdr(n).height = h
		}
	}
}
