//go:build goexperiment.simd && amd64

package hot

import "testing"

// TestSearchPathsAgreeSIMD exercises the runtime SIMD-disable wiring of a
// SIMD build (the !useSIMD branches never run on AVX2 CI machines
// otherwise): whole-tree searches must agree between both modes.
func TestSearchPathsAgreeSIMD(t *testing.T) {
	if !simdEnabled() {
		t.Skip("CPU has no AVX2")
	}
	orig := useSIMD
	defer func() { useSIMD = orig }()

	words := loadTestFile("test/assets/words.txt")[:30000]
	tr := newTree()
	for _, w := range words {
		tr.Insert(w, w)
	}
	for _, w := range words {
		useSIMD = true
		v1, f1 := tr.Search(w)
		useSIMD = false
		v2, f2 := tr.Search(w)
		if f1 != f2 || !f1 || string(v1.([]byte)) != string(v2.([]byte)) {
			t.Fatalf("SIMD/scalar disagreement for %q: (%v,%v) vs (%v,%v)", w, v1, f1, v2, f2)
		}
	}
}

// TestMatchKernelsFallback covers the kernels' scalar returns inside a SIMD
// build.
func TestMatchKernelsFallback(t *testing.T) {
	orig := useSIMD
	useSIMD = false
	defer func() { useSIMD = orig }()

	pks8 := [maxFanout]uint8{0, 0x11, 0x22}
	if got := match8(&pks8, 0x33, 3); got != 2 {
		t.Fatalf("match8 fallback: %d", got)
	}
	pks16 := [maxFanout]uint16{0, 0x1100, 0x2200}
	if got := match16(&pks16, 0x3300, 3); got != 2 {
		t.Fatalf("match16 fallback: %d", got)
	}
	pks32 := [maxFanout]uint32{0, 0x11000000, 0x22000000}
	if got := match32(&pks32, 0x33000000, 3); got != 2 {
		t.Fatalf("match32 fallback: %d", got)
	}
}
