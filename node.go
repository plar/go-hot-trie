package hot

import (
	"encoding/binary"
	"math/bits"
	"unsafe"
)

// maxFanout is k, the maximum number of entries per compound node. It is 32
// so that all 8-bit sparse partial keys of a full node can be compared with a
// single 256-bit (AVX2) SIMD instruction (paper §5.1.4).
const maxFanout = 32

// maxBits is the maximum number of discriminative bits per node (k-1).
const maxBits = maxFanout - 1

const (
	kindLeaf uint8 = iota
	kindNode8
	kindNode16
	kindNode32
)

// Extraction spec kinds (paper Fig. 14): a single 8-byte window mask, a
// gathered window of up to 8 scattered mask bytes, or per-byte offset/mask
// pairs for more than 8 scattered bytes.
const (
	specSingle uint8 = iota
	specGather
	specScatter
)

// header is embedded first in every node type so that any node pointer can be
// inspected via (*header)(p).
type header struct {
	kind   uint8
	height uint8 // compound node height; leaves have height 0
	num    uint8 // number of entries
	nBits  uint8 // number of discriminative bit positions
}

func hdr(p unsafe.Pointer) *header { return (*header)(p) }

func isLeaf(p unsafe.Pointer) bool { return hdr(p).kind == kindLeaf }

// entryHeight is the height of an entry; leaves store a truthful 0.
func entryHeight(p unsafe.Pointer) uint8 { return hdr(p).height }

// A leaf reuses the (otherwise unused) header nBits field as a "key contains
// NUL bytes" flag, letting insertion skip the NUL scan when computing
// mismatch bits against candidate leaves. Leaf heights stay truthfully 0.
type leaf struct {
	hdr   header
	key   Key
	value Value
}

func newLeaf(key Key, value Value, hasNul bool) unsafe.Pointer {
	l := &leaf{hdr: header{kind: kindLeaf}, key: key, value: value}
	if hasNul {
		l.hdr.nBits = 1
	}
	return unsafe.Pointer(l)
}

// leafEkey returns the escaped encoding of a leaf's key.
func (l *leaf) ekey() []byte {
	if l.hdr.nBits != 0 {
		return escapeKey(l.key)
	}
	return l.key
}

// pkWidth is the set of supported sparse partial key widths (paper §5.1.3:
// the Adaptive Linearized node Layout stores partial keys as the smallest of
// 8-, 16- or 32-bit integers that fits the node's discriminative bits).
type pkWidth interface{ ~uint8 | ~uint16 | ~uint32 }

// node is a compound node using the linearized node layout (paper §5.1).
//
// The structural area consists of the discriminative bit extraction spec
// (single-mask: offset+mask over an 8-byte window, or multi-mask: per-byte
// offset/mask pairs) and the sparse partial keys, one W-bit integer per
// entry, ordered by key. The data area is the children array: pointers to
// leaves or other nodes in key order.
//
// Sparse partial key convention: the bit for the node's smallest
// discriminative position (rank 0, the internal root BiNode) is stored in the
// most significant bit of W, so partial keys are strictly ascending in entry
// order. A bit is set iff the BiNode at that rank lies on the entry's path
// from the internal root and the entry is in its right (1) subtree.
type node[W pkWidth] struct {
	hdr        header
	spec       uint8 // specSingle, specGather or specScatter
	nMaskBytes uint8 // multi-mask: number of offset/mask pairs
	nRuns      uint8 // number of contiguous mask runs in the (gathered) window
	alignShift uint8 // W bit width - nBits: aligns extracted keys to the MSB
	offset     uint32
	// mask lives in the first cache line: the hardware PEXT path reads it on
	// every node visit.
	mask uint64

	// Contiguous runs of the 64-bit window mask, MSB-first: extraction is a
	// handful of shift/mask/or steps instead of a per-bit PEXT emulation.
	runShift [maxBits]uint8
	runLen   [maxBits]uint8

	partialKeys [maxFanout]W
	children    [maxFanout]unsafe.Pointer

	byteOffsets [maxBits]uint32
	byteMasks   [maxBits]uint8
}

type node8 = node[uint8]
type node16 = node[uint16]
type node32 = node[uint32]

// extract computes the dense partial key of the encoded search key: the bits
// at the node's discriminative positions, packed in position order and
// aligned to the most significant bit of W. This is the PEXT step of the
// paper's Listing 5.
func (n *node[W]) extract(ek []byte) W {
	var x uint64
	switch n.spec {
	case specSingle:
		off := int(n.offset)
		if off+8 <= len(ek) {
			x = binary.BigEndian.Uint64(ek[off:])
		} else {
			x = loadWindow(ek, off)
		}
	case specGather:
		// Gather the (at most 8) mask bytes into one big-endian window.
		for j := 0; j < int(n.nMaskBytes); j++ {
			x = x<<8 | uint64(keyByteAt(ek, int(n.byteOffsets[j])))
		}
		x <<= 8 * (8 - uint(n.nMaskBytes))
	default:
		// More than 8 scattered mask bytes: per-byte PEXT emulation.
		var v uint32
		for j := 0; j < int(n.nMaskBytes); j++ {
			b := keyByteAt(ek, int(n.byteOffsets[j]))
			m := n.byteMasks[j]
			v = v<<uint(bits.OnesCount8(m)) | uint32(pext8(b, m))
		}
		return W(v) << n.alignShift
	}
	if useBMI2 {
		// Hardware PEXT (paper Listing 5), via a small assembly routine.
		return W(pext64asm(x, n.mask)) << n.alignShift
	}
	// PEXT via precomputed contiguous mask runs (MSB-first), typically only
	// a few per node.
	v := uint32(x>>n.runShift[0]) & (1<<n.runLen[0] - 1)
	for j := 1; j < int(n.nRuns); j++ {
		l := n.runLen[j]
		v = v<<l | uint32(x>>n.runShift[j])&(1<<l-1)
	}
	return W(v) << n.alignShift
}

// loadWindow loads 8 encoded-key bytes starting at off, big-endian,
// serving the virtual terminator and zero padding past the key's end.
func loadWindow(ek []byte, off int) uint64 {
	rem := len(ek) - off
	var x uint64
	switch {
	case rem >= 8:
		return binary.BigEndian.Uint64(ek[off:])
	case rem > 0 && len(ek) >= 8:
		// Overlapping backward load: the wanted bytes are the low rem bytes.
		x = binary.BigEndian.Uint64(ek[len(ek)-8:]) << (8 * uint(8-rem))
	case rem > 0:
		for i := range rem {
			x |= uint64(ek[off+i]) << (8 * uint(7-i))
		}
	}
	if t := rem + 1; uint(t) < 8 { // the 0x01 terminator byte, if in window
		x |= 1 << (8 * uint(7-t))
	}
	return x
}

// pext8 packs the bits of b selected by mask, preserving order (the most
// significant selected bit of b becomes the most significant result bit).
func pext8(b, mask uint8) uint8 {
	var r uint8
	for mask != 0 {
		hi := uint8(0x80) >> bits.LeadingZeros8(mask)
		r <<= 1
		if b&hi != 0 {
			r |= 1
		}
		mask ^= hi
	}
	return r
}

// positions appends the node's discriminative bit positions in ascending
// order to buf and returns the filled slice.
func (n *node[W]) positions(buf *[maxBits]uint32) []uint32 {
	ps := buf[:0]
	if n.spec == specSingle {
		m := n.mask
		base := n.offset * 8
		for m != 0 {
			b := bits.LeadingZeros64(m)
			ps = append(ps, base+uint32(b))
			m &^= 1 << (63 - b)
		}
	} else {
		for j := 0; j < int(n.nMaskBytes); j++ {
			base := n.byteOffsets[j] * 8
			m := n.byteMasks[j]
			for m != 0 {
				b := bits.LeadingZeros8(m)
				ps = append(ps, base+uint32(b))
				m &^= 1 << (7 - b)
			}
		}
	}
	return ps
}

// setSpec compiles the sorted position list into the extraction spec: a
// single 8-byte window mask when all positions fit in one window, otherwise
// per-byte offset/mask pairs (paper Fig. 14).
func (n *node[W]) setSpec(ps []uint32) {
	n.hdr.nBits = uint8(len(ps))
	n.alignShift = uint8(wBits[W]()) - n.hdr.nBits
	first := ps[0] / 8
	last := ps[len(ps)-1] / 8
	if last-first < 8 {
		n.spec = specSingle
		n.offset = first
		n.mask = 0
		for _, p := range ps {
			n.mask |= 1 << (63 - (p - first*8))
		}
		n.compileRuns(n.mask)
	} else {
		k := 0
		for i := 0; i < len(ps); {
			bo := ps[i] / 8
			var m uint8
			for i < len(ps) && ps[i]/8 == bo {
				m |= 1 << (7 - ps[i]%8)
				i++
			}
			n.byteOffsets[k] = bo
			n.byteMasks[k] = m
			k++
		}
		n.nMaskBytes = uint8(k)
		if k <= 8 {
			n.spec = specGather
			// Compile runs over the synthesized window of gathered bytes;
			// the synthesized mask also feeds the hardware PEXT path.
			var gm uint64
			for j := 0; j < k; j++ {
				gm |= uint64(n.byteMasks[j]) << (56 - 8*j)
			}
			n.mask = gm
			n.compileRuns(gm)
		} else {
			n.spec = specScatter
		}
	}
}

// compileRuns splits a 64-bit window mask into contiguous runs of set bits
// (MSB-first), stored as (right-shift, length) pairs.
func (n *node[W]) compileRuns(m uint64) {
	k := 0
	for m != 0 {
		lead := bits.LeadingZeros64(m)
		runLen := bits.LeadingZeros64(^(m << lead))
		shift := 64 - lead - runLen
		n.runShift[k] = uint8(shift)
		n.runLen[k] = uint8(runLen)
		k++
		m &^= (1<<runLen - 1) << shift
	}
	n.nRuns = uint8(k)
}

// childrenOf returns the node's live children slice, dispatching on the
// kind once per node instead of once per child.
func childrenOf(p unsafe.Pointer) []unsafe.Pointer {
	switch hdr(p).kind {
	case kindNode8:
		n := (*node8)(p)
		return n.children[:n.hdr.num]
	case kindNode16:
		n := (*node16)(p)
		return n.children[:n.hdr.num]
	default:
		n := (*node32)(p)
		return n.children[:n.hdr.num]
	}
}

func childAt(p unsafe.Pointer, i int) unsafe.Pointer {
	switch hdr(p).kind {
	case kindNode8:
		return (*node8)(p).children[i]
	case kindNode16:
		return (*node16)(p).children[i]
	default:
		return (*node32)(p).children[i]
	}
}

func setChildAt(p unsafe.Pointer, i int, c unsafe.Pointer) {
	switch hdr(p).kind {
	case kindNode8:
		(*node8)(p).children[i] = c
	case kindNode16:
		(*node16)(p).children[i] = c
	default:
		(*node32)(p).children[i] = c
	}
}

// computeHeight recomputes a node's height from its entries.
func computeHeight(p unsafe.Pointer) uint8 {
	var m uint8
	for _, c := range childrenOf(p) {
		m = max(m, entryHeight(c))
	}
	return m + 1
}

// Node interface implementation, so that leaves and nodes can be handed to
// traversal callbacks without extra allocations.

func (l *leaf) Kind() Kind   { return Leaf }
func (l *leaf) Key() Key     { return l.key }
func (l *leaf) Value() Value { return l.value }

func (n *node[W]) Kind() Kind {
	switch n.hdr.kind {
	case kindNode8:
		return Node8
	case kindNode16:
		return Node16
	default:
		return Node32
	}
}
func (n *node[W]) Key() Key     { return nil }
func (n *node[W]) Value() Value { return nil }

// asNode converts an internal pointer to the public Node interface.
func asNode(p unsafe.Pointer) Node {
	switch hdr(p).kind {
	case kindLeaf:
		return (*leaf)(p)
	case kindNode8:
		return (*node8)(p)
	case kindNode16:
		return (*node16)(p)
	default:
		return (*node32)(p)
	}
}
