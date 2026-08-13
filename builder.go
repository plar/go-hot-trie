package hot

import (
	"math/bits"
	"slices"
	"unsafe"
)

// nb is a node builder: an explicit, fixed-capacity representation of a
// compound node used for structural modifications (insertion of a
// discriminating BiNode, node splits, merges and BiNode pull-down). Sparse
// partial keys are held MSB-aligned in 32 bits regardless of the final node
// width; one extra slot beyond the maximum fanout accommodates the transient
// overflow state before a split.
//
// All builders live on the stack; only materialize allocates.
type nb struct {
	np        int // number of positions
	ne        int // number of entries
	positions [maxBits + 1]uint32
	sparse    [maxFanout + 1]uint32
	children  [maxFanout + 1]unsafe.Pointer
}

// load fills the builder from an existing node.
func load(p unsafe.Pointer, b *nb) {
	switch hdr(p).kind {
	case kindNode8:
		loadFrom[uint8](p, b)
	case kindNode16:
		loadFrom[uint16](p, b)
	default:
		loadFrom[uint32](p, b)
	}
}

func loadFrom[W pkWidth](p unsafe.Pointer, b *nb) {
	n := (*node[W])(p)
	shift := 32 - wBits[W]()
	var buf [maxBits]uint32
	ps := n.positions(&buf)
	b.np = copy(b.positions[:len(ps)], ps)
	b.ne = int(n.hdr.num)
	for i := 0; i < b.ne; i++ {
		b.sparse[i] = uint32(n.partialKeys[i]) << shift
		b.children[i] = n.children[i]
	}
	// Reclaim positions left behind by lazy in-place deletion so that
	// structural operations always start from exact sparse key semantics
	// and minimal position lists.
	b.dropUnusedPositions()
}

// materialize allocates a node of the smallest fitting partial key width.
func (b *nb) materialize() unsafe.Pointer {
	switch {
	case b.np <= 8:
		return mat[uint8](b, kindNode8)
	case b.np <= 16:
		return mat[uint16](b, kindNode16)
	default:
		return mat[uint32](b, kindNode32)
	}
}

func mat[W pkWidth](b *nb, kind uint8) unsafe.Pointer {
	shift := 32 - wBits[W]()
	n := &node[W]{}
	n.hdr.kind = kind
	n.hdr.num = uint8(b.ne)
	var maxH uint8
	for i := 0; i < b.ne; i++ {
		n.partialKeys[i] = W(b.sparse[i] >> shift)
		n.children[i] = b.children[i]
		maxH = max(maxH, entryHeight(b.children[i]))
	}
	n.hdr.height = maxH + 1
	n.setSpec(b.positions[:b.np])
	return unsafe.Pointer(n)
}

// rankBit is the sparse key bit for rank r (rank 0 = most significant).
func rankBit(r int) uint32 { return 1 << (31 - r) }

// prefixMask selects the bits of all ranks strictly below r.
func prefixMask(r int) uint32 {
	return ^uint32(0) << (32 - r) // r==0 yields 0 (Go defines the shift)
}

// ensurePosition returns the rank of pos, inserting it into the position
// list (recoding all sparse partial keys with a zero bit at the new rank,
// the PDEP step of paper §5.1.2) if it is not present yet.
func (b *nb) ensurePosition(pos uint32) int {
	r, found := slices.BinarySearch(b.positions[:b.np], pos)
	if found {
		return r
	}
	copy(b.positions[r+1:b.np+1], b.positions[r:b.np])
	b.positions[r] = pos
	b.np++
	hi := prefixMask(r)
	for i := 0; i < b.ne; i++ {
		s := b.sparse[i]
		b.sparse[i] = s&hi | s&^hi>>1
	}
	return r
}

// affectedRange returns the contiguous range [lo,hi] of entries whose sparse
// partial keys agree with entry c on all ranks below r: the entries of the
// subtree rooted at the mismatching BiNode.
func (b *nb) affectedRange(c, r int) (int, int) {
	pm := prefixMask(r)
	pfx := b.sparse[c] & pm
	lo, hi := c, c
	for lo > 0 && b.sparse[lo-1]&pm == pfx {
		lo--
	}
	for hi+1 < b.ne && b.sparse[hi+1]&pm == pfx {
		hi++
	}
	return lo, hi
}

// insertEntry inserts a new child discriminated from the subtree around
// entry c by the bit at position mbit (whose value in the new key is newBit).
// This is the "normal insert" of paper §3.2.1 performed directly on the
// linearized representation (§5.1.1). The node may transiently overflow to
// maxFanout+1 entries; the caller handles the split.
func (b *nb) insertEntry(mbit uint32, newBit uint32, c int, child unsafe.Pointer) {
	r := b.ensurePosition(mbit)
	lo, hi := b.affectedRange(c, r)
	pfx := b.sparse[c] & prefixMask(r)
	var at int
	var s uint32
	if newBit == 1 {
		// New key goes right of the affected subtree; affected entries keep
		// a zero at the new rank.
		at = hi + 1
		s = pfx | rankBit(r)
	} else {
		// New key goes left; the affected subtree becomes the right child of
		// the new BiNode.
		rb := rankBit(r)
		for i := lo; i <= hi; i++ {
			b.sparse[i] |= rb
		}
		at = lo
		s = pfx
	}
	b.splice(at, s, child)
}

// splice inserts one entry at index at.
func (b *nb) splice(at int, s uint32, child unsafe.Pointer) {
	copy(b.sparse[at+1:b.ne+1], b.sparse[at:b.ne])
	copy(b.children[at+1:b.ne+1], b.children[at:b.ne])
	b.sparse[at] = s
	b.children[at] = child
	b.ne++
}

// replaceWithPair replaces entry j with two entries (l, r) discriminated by
// the bit at position q. Used by parent pull-up (paper §3.2.1): l and r are
// the two halves of a split child and q is the split child's root BiNode
// position.
func (b *nb) replaceWithPair(j int, q uint32, l, r unsafe.Pointer) {
	rq := b.ensurePosition(q)
	b.children[j] = l
	b.splice(j+1, b.sparse[j]|rankBit(rq), r)
}

// compressSparse packs the bits of s selected by mask (both MSB-aligned in
// 32 bits), preserving order and re-aligning the result to the MSB.
func compressSparse(s, mask uint32) uint32 {
	var r uint32
	m := mask
	for m != 0 {
		lz := bits.LeadingZeros32(m)
		r = r<<1 | s>>(31-lz)&1
		m &^= 1 << (31 - lz)
	}
	return r << (32 - bits.OnesCount32(mask))
}

// dropUnusedPositions removes discriminative positions that no longer have a
// BiNode in the node: a rank is used iff some entry has a one bit at it
// (every BiNode has a right subtree whose entries carry its one bit).
func (b *nb) dropUnusedPositions() {
	var or uint32
	for i := 0; i < b.ne; i++ {
		or |= b.sparse[i]
	}
	used := or & (^uint32(0) << (32 - b.np))
	if bits.OnesCount32(used) == b.np {
		return
	}
	for i := 0; i < b.ne; i++ {
		b.sparse[i] = compressSparse(b.sparse[i], used)
	}
	k := 0
	for r := 0; r < b.np; r++ {
		if used&rankBit(r) != 0 {
			b.positions[k] = b.positions[r]
			k++
		}
	}
	b.np = k
}

// split divides an overflowing builder at its internal root BiNode (rank 0)
// into two sides and returns the resulting entries (single-entry sides are
// hoisted to the entry itself, per HOT's leaf hoisting) along with the root
// BiNode position q.
func (b *nb) split() (l, r unsafe.Pointer, q uint32) {
	q = b.positions[0]
	msb := rankBit(0)
	m := 0
	for m < b.ne && b.sparse[m]&msb == 0 {
		m++
	}
	l = b.buildSide(0, m)
	r = b.buildSide(m, b.ne)
	return l, r, q
}

// buildSide materializes entries [lo,hi) as one side of a split: the rank-0
// bit is dropped and unused positions are removed.
func (b *nb) buildSide(lo, hi int) unsafe.Pointer {
	if hi-lo == 1 {
		return b.children[lo]
	}
	var s nb
	s.np = copy(s.positions[:b.np-1], b.positions[1:b.np])
	s.ne = hi - lo
	for i := lo; i < hi; i++ {
		s.sparse[i-lo] = b.sparse[i] << 1
		s.children[i-lo] = b.children[i]
	}
	s.dropUnusedPositions()
	return s.materialize()
}

// mergeNodes combines two sibling nodes l and r under their parent BiNode at
// position q into one node (the inverse of parent pull-up, used by
// deletion's node merge).
func mergeNodes(l, r unsafe.Pointer, q uint32) unsafe.Pointer {
	var lb, rb, out nb
	load(l, &lb)
	load(r, &rb)
	out.positions[0] = q
	// q is an ancestor position of both sides, so it takes rank 0; the
	// merged tail starts at rank 1, which the rank maps account for.
	var lmap, rmap [maxBits]int
	out.np = 1 + mergePositions(out.positions[1:], lb.positions[:lb.np], rb.positions[:rb.np], lmap[:], rmap[:])
	for i := 0; i < lb.ne; i++ {
		out.sparse[out.ne] = remapSparse(lb.sparse[i], lmap[:])
		out.children[out.ne] = lb.children[i]
		out.ne++
	}
	for i := 0; i < rb.ne; i++ {
		out.sparse[out.ne] = remapSparse(rb.sparse[i], rmap[:]) | rankBit(0)
		out.children[out.ne] = rb.children[i]
		out.ne++
	}
	return out.materialize()
}

// mergePositions merges two sorted position lists into dst, collapsing
// duplicates (the same position can host BiNodes in both subtrees), and
// records each source rank's destination rank plus one (accounting for the
// parent BiNode at rank 0) in the rank maps. Returns the merged length.
func mergePositions(dst []uint32, a, b []uint32, amap, bmap []int) int {
	i, j, k := 0, 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case j == len(b) || (i < len(a) && a[i] < b[j]):
			dst[k] = a[i]
			amap[i] = k + 1
			i++
		case i == len(a) || b[j] < a[i]:
			dst[k] = b[j]
			bmap[j] = k + 1
			j++
		default: // equal
			dst[k] = a[i]
			amap[i] = k + 1
			bmap[j] = k + 1
			i++
			j++
		}
		k++
	}
	return k
}

// remapSparse translates a sparse key through a source-rank to
// destination-rank map.
func remapSparse(s uint32, rankMap []int) uint32 {
	var r uint32
	for s != 0 {
		lz := bits.LeadingZeros32(s)
		r |= rankBit(rankMap[lz])
		s &^= 1 << (31 - lz)
	}
	return r
}

// absorb pulls the parent BiNode at position q and the sibling entry s down
// into node n (deletion's simple BiNode pull-down). sLeft tells whether the
// sibling is the left child of the BiNode.
func absorb(n unsafe.Pointer, q uint32, s unsafe.Pointer, sLeft bool) unsafe.Pointer {
	var b nb
	load(n, &b)
	// q is an ancestor position of everything in n: it becomes rank 0.
	copy(b.positions[1:b.np+1], b.positions[:b.np])
	b.positions[0] = q
	b.np++
	rbit := rankBit(0)
	for i := 0; i < b.ne; i++ {
		b.sparse[i] >>= 1
	}
	if sLeft {
		copy(b.sparse[1:b.ne+1], b.sparse[:b.ne])
		copy(b.children[1:b.ne+1], b.children[:b.ne])
		for i := 1; i <= b.ne; i++ {
			b.sparse[i] |= rbit
		}
		b.sparse[0] = 0
		b.children[0] = s
	} else {
		b.sparse[b.ne] = rbit
		b.children[b.ne] = s
	}
	b.ne++
	return b.materialize()
}
