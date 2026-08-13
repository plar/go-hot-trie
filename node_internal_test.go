package hot

import (
	"math/rand"
	"slices"
	"testing"
)

// randPositions builds a sorted list of n distinct bit positions whose byte
// spread is controlled: spreadBytes 1..7 keeps them in one window
// (specSingle), 2..8 distinct bytes over a wide range forces specGather,
// more forces specScatter.
func randPositions(rng *rand.Rand, n, nBytes, byteStride int) []uint32 {
	n = min(n, nBytes*8) // cannot ask for more distinct bits than exist
	byteOffs := make([]int, nBytes)
	off := rng.Intn(4)
	for i := range byteOffs {
		byteOffs[i] = off
		off += 1 + rng.Intn(byteStride)
	}
	seen := map[uint32]bool{}
	var ps []uint32
	for len(ps) < n {
		p := uint32(byteOffs[rng.Intn(nBytes)]*8 + rng.Intn(8))
		if !seen[p] {
			seen[p] = true
			ps = append(ps, p)
		}
	}
	slices.Sort(ps)
	return ps
}

// refExtract collects the discriminative bits of ek one at a time.
func refExtract(ek []byte, ps []uint32) uint32 {
	var v uint32
	for _, p := range ps {
		v = v<<1 | keyBitAt(ek, p)
	}
	return v
}

// TestExtractMatchesKeyBitAt verifies the compiled extraction specs (single
// window, gathered window, scattered bytes, run compilation, hardware PEXT
// when available) against the per-bit reference for all three widths.
func TestExtractMatchesKeyBitAt(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	check := func(ps []uint32, wantSpec uint8) {
		t.Helper()
		nBits := len(ps)
		for range 50 {
			ek := make([]byte, int(ps[len(ps)-1]/8)+1+rng.Intn(4))
			rng.Read(ek)
			if rng.Intn(3) == 0 { // short keys exercise the virtual terminator
				ek = ek[:rng.Intn(len(ek))]
			}
			want := refExtract(ek, ps)
			switch {
			case nBits <= 8:
				n := &node8{}
				n.setSpec(ps)
				if n.spec != wantSpec {
					t.Fatalf("spec=%d want %d for %v", n.spec, wantSpec, ps)
				}
				if got := n.extract(ek); got != uint8(want)<<n.alignShift {
					t.Fatalf("extract8(%x, %v) = %08b want %08b", ek, ps, got, uint8(want)<<n.alignShift)
				}
			case nBits <= 16:
				n := &node16{}
				n.setSpec(ps)
				if got := n.extract(ek); got != uint16(want)<<n.alignShift {
					t.Fatalf("extract16(%x, %v) = %016b want %016b", ek, ps, got, uint16(want)<<n.alignShift)
				}
			default:
				n := &node32{}
				n.setSpec(ps)
				if got := n.extract(ek); got != want<<n.alignShift {
					t.Fatalf("extract32(%x, %v) = %032b want %032b", ek, ps, got, want<<n.alignShift)
				}
			}
		}
	}

	for range 200 {
		// Single window: up to 8 bits in <= 8 consecutive-ish bytes.
		check(randPositions(rng, 2+rng.Intn(7), 1+rng.Intn(3), 1), specSingle)
	}
	for range 200 {
		// Gathered: 2..8 distinct bytes spread past one window.
		nb := 2 + rng.Intn(7)
		ps := randPositions(rng, max(nb, 2+rng.Intn(15)), nb, 20)
		n := &node32{}
		n.setSpec(ps)
		if int(ps[len(ps)-1]/8-ps[0]/8) >= 8 && n.spec == specSingle {
			t.Fatalf("expected non-single spec for %v", ps)
		}
		check(ps, n.spec)
	}
	for range 100 {
		// Scattered: more than 8 distinct bytes.
		nb := 9 + rng.Intn(10)
		check(randPositions(rng, max(nb, 10+rng.Intn(20)), nb, 15), specScatter)
	}
}

// TestSpecGatherScatterBoundary pins the spec choice at exactly 8 vs 9
// distinct mask bytes.
func TestSpecGatherScatterBoundary(t *testing.T) {
	mk := func(nBytes int) []uint32 {
		ps := make([]uint32, nBytes)
		for i := range ps {
			ps[i] = uint32(i * 80) // one bit per byte, 10 bytes apart
		}
		return ps
	}
	n8 := &node8{}
	n8.setSpec(mk(8))
	if n8.spec != specGather {
		t.Fatalf("8 mask bytes: spec=%d want gather", n8.spec)
	}
	n16 := &node16{}
	n16.setSpec(mk(9))
	if n16.spec != specScatter {
		t.Fatalf("9 mask bytes: spec=%d want scatter", n16.spec)
	}
}

// TestSetSpecPositionsRoundTrip: positions() must invert setSpec for every
// spec kind.
func TestSetSpecPositionsRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(12))
	for i := range 500 {
		nb := 1 + rng.Intn(20)
		ps := randPositions(rng, max(nb, 1+rng.Intn(31)), nb, 1+i%25)
		n := &node32{}
		n.setSpec(ps)
		var buf [maxBits]uint32
		if got := n.positions(&buf); !slices.Equal(got, ps) {
			t.Fatalf("round trip: %v -> %v (spec=%d)", ps, got, n.spec)
		}
	}
}

// TestCompressSparseRef verifies the run-based compressSparse against a
// naive per-bit reference.
func TestCompressSparseRef(t *testing.T) {
	naive := func(s, mask uint32) uint32 {
		var r uint32
		n := 0
		for b := 31; b >= 0; b-- {
			if mask&(1<<b) != 0 {
				r = r<<1 | (s>>b)&1
				n++
			}
		}
		return r << (32 - n)
	}
	rng := rand.New(rand.NewSource(13))
	for range 100000 {
		s := rng.Uint32()
		mask := rng.Uint32() | 1<<31 // MSB-aligned masks in practice
		if got, want := compressSparse(s, mask), naive(s, mask); got != want {
			t.Fatalf("compressSparse(%08x, %08x) = %08x want %08x", s, mask, got, want)
		}
	}
}

// TestLoadWindowRef verifies loadWindow (including the overlapping backward
// load, the in-bounds branch and terminator injection) against byte-wise
// assembly from keyByteAt, for every offset around the key end.
func TestLoadWindowRef(t *testing.T) {
	rng := rand.New(rand.NewSource(14))
	for range 2000 {
		ek := make([]byte, rng.Intn(20))
		rng.Read(ek)
		for off := 0; off <= len(ek)+3; off++ {
			var want uint64
			for j := range 8 {
				want = want<<8 | uint64(keyByteAt(ek, off+j))
			}
			if got := loadWindow(ek, off); got != want {
				t.Fatalf("loadWindow(%x, %d) = %016x want %016x", ek, off, got, want)
			}
		}
	}
}

// TestBuilderWidthReclamation: after lazy deletions leave dead positions, a
// builder round-trip must reclaim them and pick the smaller width.
func TestBuilderWidthReclamation(t *testing.T) {
	tr := newTree()
	// 32 keys, each with a single distinct bit set across 4 bytes: 32
	// discriminative positions force a Node32 root.
	var keys []Key
	for i := range 32 {
		k := make([]byte, 4)
		k[i/8] = 1 << (7 - i%8)
		keys = append(keys, k)
	}
	for _, k := range keys {
		tr.Insert(k, string(k))
	}
	if rootKind(tr) != Node32 {
		t.Fatalf("root kind %v, want Node32", rootKind(tr))
	}
	// Delete all but the first 5 keys: in-place removal leaves dead
	// positions and the root stays Node32 by design.
	for _, k := range keys[5:] {
		tr.Delete(k)
	}
	checkInvariants(t, tr)
	if rootKind(tr) != Node32 {
		t.Fatalf("lazy deletion changed root kind to %v", rootKind(tr))
	}
	// A builder round-trip reclaims the dead positions and shrinks the width.
	var b nb
	load(tr.root, &b)
	if b.np >= 8 {
		t.Fatalf("expected < 8 live positions, got %d", b.np)
	}
	rebuilt := b.materialize()
	if got := asNode(rebuilt).Kind(); got != Node8 {
		t.Fatalf("materialize kind %v, want Node8", got)
	}
	tr.root = rebuilt
	checkInvariants(t, tr)
	for _, k := range keys[:5] {
		if _, found := tr.Search(k); !found {
			t.Fatalf("key %x lost after reclamation", k)
		}
	}
}
