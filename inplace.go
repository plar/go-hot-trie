package hot

import (
	"math/bits"
	"unsafe"
)

// In-place fast paths for the normal insert and normal delete cases. They
// mirror the builder's insertEntry / removeEntryCore but mutate the node's
// W-width arrays directly, avoiding the builder round-trip and reallocation.
// Structural cases (overflow split, partial key width upgrades, underflow
// merges) still go through the builder.

func wBits[W pkWidth]() uint { return uint(unsafe.Sizeof(W(0))) * 8 }

// diffRankW is the rank of the BiNode discriminating two distinct entries of
// the same node: the most significant differing sparse key bit.
func diffRankW[W pkWidth](x, y W) int {
	return bits.LeadingZeros32(uint32(x^y) << (32 - wBits[W]()))
}

// neighborDiffRanks returns the diff ranks of entry c against its left and
// right neighbors (-1 when absent). The parent BiNode of entry c is the
// deeper (larger) of the two; c is its left child iff dr > dl.
func neighborDiffRanks[W pkWidth](pks []W, c int) (dl, dr int) {
	dl, dr = -1, -1
	if c > 0 {
		dl = diffRankW(pks[c-1], pks[c])
	}
	if c+1 < len(pks) {
		dr = diffRankW(pks[c], pks[c+1])
	}
	return
}

// removeEntryCore removes entry c and its discriminating BiNode from
// parallel sparse-key/children slices (both of length = entry count). If c
// was the left child of its parent BiNode, the hoisted right sibling subtree
// drops its now-stale one bit so the sparse key semantics stay exact.
func removeEntryCore[W pkWidth](pks []W, children []unsafe.Pointer, c int) {
	num := len(pks)
	if num > 1 {
		dl, dr := neighborDiffRanks(pks, c)
		if dr > dl {
			wb := wBits[W]()
			pm := ^W(0) << (wb - uint(dr))
			pfx := pks[c] & pm
			rb := W(1) << (wb - 1 - uint(dr))
			for i := c + 1; i < num && pks[i]&pm == pfx; i++ {
				pks[i] &^= rb
			}
		}
	}
	copy(pks[c:num-1], pks[c+1:num])
	copy(children[c:num-1], children[c+1:num])
}

// ipInsert inserts a new child discriminated at mbit around entry c,
// mutating the node in place. It returns false when the node is full or the
// new position does not fit the current partial key width.
func ipInsert[W pkWidth](n *node[W], c int, mbit, newBit uint32, child unsafe.Pointer) bool {
	num := int(n.hdr.num)
	if num >= maxFanout {
		return false
	}
	wb := wBits[W]()
	var r int
	var exists bool
	if n.spec == specSingle && mbit >= n.offset*8 && mbit < n.offset*8+64 {
		// Fast path: the rank of a window bit is the number of set mask bits
		// above it, with no need to decode the position list.
		idx := mbit - n.offset*8
		r = bits.OnesCount64(n.mask >> (64 - idx))
		exists = n.mask&(1<<(63-idx)) != 0
		if !exists {
			if uint(n.hdr.nBits)+1 > wb {
				return false // needs a wider partial key type
			}
			n.mask |= 1 << (63 - idx)
			n.compileRuns(n.mask)
			n.hdr.nBits++
			n.alignShift--
		}
	} else {
		var buf [maxBits]uint32
		ps := n.positions(&buf)
		for r < len(ps) && ps[r] < mbit {
			r++
		}
		exists = r < len(ps) && ps[r] == mbit
		if !exists {
			if uint(len(ps))+1 > wb {
				return false // needs a wider partial key type
			}
			ps = append(ps, 0)
			copy(ps[r+1:], ps[r:])
			ps[r] = mbit
			n.setSpec(ps)
		}
	}
	if !exists {
		// Recode: insert a zero bit at rank r in every sparse partial key.
		hm := ^W(0) << (wb - uint(r))
		for j := range num {
			s := n.partialKeys[j]
			n.partialKeys[j] = s&hm | s&^hm>>1
		}
	}
	hm := ^W(0) << (wb - uint(r))
	pfx := n.partialKeys[c] & hm
	lo, hi := c, c
	for lo > 0 && n.partialKeys[lo-1]&hm == pfx {
		lo--
	}
	for hi+1 < num && n.partialKeys[hi+1]&hm == pfx {
		hi++
	}
	rb := W(1) << (wb - 1 - uint(r))
	var at int
	var s W
	if newBit == 1 {
		at = hi + 1
		s = pfx | rb
	} else {
		for j := lo; j <= hi; j++ {
			n.partialKeys[j] |= rb
		}
		at = lo
		s = pfx
	}
	copy(n.partialKeys[at+1:num+1], n.partialKeys[at:num])
	copy(n.children[at+1:num+1], n.children[at:num])
	n.partialKeys[at] = s
	n.children[at] = child
	n.hdr.num++
	return true
}

// ipRemove removes entry c and its discriminating BiNode in place. The
// caller guarantees at least 3 entries. Positions whose BiNode vanished are
// left in place: with all sparse bits at the dead rank zero they cannot
// influence matching, and they are reclaimed by the next structural
// modification (load() drops them), keeping the normal delete path cheap.
func ipRemove[W pkWidth](n *node[W], c int) {
	num := int(n.hdr.num)
	removeEntryCore(n.partialKeys[:num], n.children[:num], c)
	n.hdr.num--
}

// ipRemoveAt dispatches ipRemove on the node kind.
func ipRemoveAt(p unsafe.Pointer, c int) {
	switch hdr(p).kind {
	case kindNode8:
		ipRemove((*node8)(p), c)
	case kindNode16:
		ipRemove((*node16)(p), c)
	default:
		ipRemove((*node32)(p), c)
	}
}
