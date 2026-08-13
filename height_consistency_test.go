package hot

import (
	"math/rand"
	"testing"
	"unsafe"
)

// TestHeightConsistencyUnderDeletion deletes random deep trees key by key
// and scans the whole tree for stale stored heights after every delete.
func TestHeightConsistencyUnderDeletion(t *testing.T) {
	for seed := int64(0); seed < 25; seed++ {
		rng := rand.New(rand.NewSource(seed))
		tr := newTree()
		var keys []Key
		seen := map[string]bool{}
		n := 200 + rng.Intn(1200)
		for len(keys) < n {
			l := 1 + rng.Intn(30)
			k := make([]byte, l)
			for j := range k {
				k[j] = "ab"[rng.Intn(2)] // dense shared prefixes -> deep trees
			}
			if !seen[string(k)] {
				seen[string(k)] = true
				keys = append(keys, k)
			}
		}
		for _, k := range keys {
			tr.Insert(k, 0)
		}
		rng.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
		for di, k := range keys {
			tr.Delete(k)
			if tr.root != nil && !isLeaf(tr.root) && staleHeight(tr.root) {
				t.Fatalf("seed %d: stale height after delete %d/%d", seed, di, len(keys))
			}
		}
	}
}

// staleHeight is the cheap height-only subset of checkNode, for per-delete
// loops where the full invariant checker would be too slow.
func staleHeight(p unsafe.Pointer) bool {
	if isLeaf(p) {
		return false
	}
	if computeHeight(p) != hdr(p).height {
		return true
	}
	for _, c := range childrenOf(p) {
		if staleHeight(c) {
			return true
		}
	}
	return false
}

// TestDatasetInvariants runs the structural invariant checker over the
// bundled dataset trees.
func TestDatasetInvariants(t *testing.T) {
	tr, _ := sharedWordsTree()
	checkInvariants(t, tr)
	for ds, want := range map[string]treeStats{
		"test/assets/uuid.txt":      uuidStats,
		"test/assets/hsk_words.txt": hskStats,
	} {
		tr, _ := treeWithData(ds)
		checkInvariants(t, tr)
		if got := collectStats(tr.Iterator(TraverseAll)); got != want {
			t.Fatalf("%s: stats %+v want %+v", ds, got, want)
		}
	}
}
