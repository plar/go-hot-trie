//go:build amd64

package hot

import (
	"math/rand"
	"testing"
)

// pext64ref is the textbook software PEXT used as the test oracle.
func pext64ref(x, mask uint64) uint64 {
	var r uint64
	var i uint
	for mask != 0 {
		lsb := mask & -mask
		if x&lsb != 0 {
			r |= 1 << i
		}
		mask ^= lsb
		i++
	}
	return r
}

func TestPext64Asm(t *testing.T) {
	if !hasBMI2 {
		t.Skip("CPU has no BMI2")
	}
	rng := rand.New(rand.NewSource(3))
	for range 100000 {
		x := rng.Uint64()
		mask := rng.Uint64() & rng.Uint64() // varying densities
		if got, want := pext64asm(x, mask), pext64ref(x, mask); got != want {
			t.Fatalf("pext64asm(%016x, %016x) = %016x, want %016x", x, mask, got, want)
		}
	}
	// Edge cases.
	for _, mask := range []uint64{0, 1, 1 << 63, ^uint64(0)} {
		x := rng.Uint64()
		if got, want := pext64asm(x, mask), pext64ref(x, mask); got != want {
			t.Fatalf("pext64asm(%016x, %016x) = %016x, want %016x", x, mask, got, want)
		}
	}
}

// TestExtractPathsAgree verifies that hardware PEXT extraction and the
// mask-run emulation compute identical dense partial keys over real trees.
func TestExtractPathsAgree(t *testing.T) {
	if !hasBMI2 {
		t.Skip("CPU has no BMI2")
	}
	orig := useBMI2
	defer func() { useBMI2 = orig }()
	words := loadTestFile("test/assets/words.txt")[:20000]
	tr := newTree()
	for _, w := range words {
		tr.Insert(w, w)
	}
	rng := rand.New(rand.NewSource(4))
	for range 5000 {
		w := words[rng.Intn(len(words))]
		useBMI2 = false
		v1, f1 := tr.Search(w)
		useBMI2 = true
		v2, f2 := tr.Search(w)
		if f1 != f2 || string(v1.([]byte)) != string(v2.([]byte)) {
			t.Fatalf("search disagreement for %q: (%v,%v) vs (%v,%v)", w, v1, f1, v2, f2)
		}
	}
}
