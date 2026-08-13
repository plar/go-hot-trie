package hot

import (
	"bytes"
	"fmt"
	"math/rand"
	"slices"
	"testing"
)

// This test verifies the paper's central property (§4, Theorem: Minimum
// Height): for any key set, the height of the HOT tree equals the height of
// the Static Minimum Height Partitioning (Kovács & Kis) of the underlying
// binary Patricia trie with the maximum intrapartition weight constraint
// W = k-1. It also re-checks determinism via the structural invariants.

// refTrie is a reference binary Patricia trie node.
type refTrie struct {
	leaf        bool
	left, right *refTrie
}

// buildRefTrie builds a binary Patricia trie over sorted, distinct encoded
// keys.
func buildRefTrie(keys [][]byte) *refTrie {
	if len(keys) == 1 {
		return &refTrie{leaf: true}
	}
	// The root discriminating bit is the smallest position where the sorted
	// set diverges: the minimum mismatch position over adjacent pairs.
	p := -1
	at := 0
	for i := 1; i < len(keys); i++ {
		m := mismatchBit(keys[i-1], keys[i])
		if p == -1 || m < p {
			p = m
		}
	}
	// Split at the first key whose bit at p is 1.
	for at = 0; at < len(keys); at++ {
		if keyBitAt(keys[at], uint32(p)) == 1 {
			break
		}
	}
	return &refTrie{
		left:  buildRefTrie(keys[:at]),
		right: buildRefTrie(keys[at:]),
	}
}

// smhp labels the trie bottom-up per Kovács & Kis with weight 0 for leaves,
// 1 for BiNodes, and the given maximum intrapartition weight, returning
// (level, intrapartition weight).
func smhp(n *refTrie, maxW int) (int, int) {
	if n.leaf {
		return 0, 0
	}
	ll, lw := smhp(n.left, maxW)
	rl, rw := smhp(n.right, maxW)
	clmax := max(ll, rl)
	sum := 1
	if ll == clmax {
		sum += lw
	}
	if rl == clmax {
		sum += rw
	}
	if sum <= maxW {
		return clmax, sum
	}
	return clmax + 1, 1
}

func checkMinHeight(t *testing.T, keys []string) {
	t.Helper()
	tr := newTree()
	for _, k := range keys {
		tr.Insert(Key(k), k)
	}
	sorted := make([][]byte, 0, len(keys))
	for _, k := range keys {
		sorted = append(sorted, escapeKey(Key(k)))
	}
	slices.SortFunc(sorted, bytes.Compare)
	level, _ := smhp(buildRefTrie(sorted), maxFanout-1)
	want := uint8(level + 1)
	if got := hdr(tr.root).height; got != want {
		t.Fatalf("%d keys: HOT height %d != minimum height %d", len(keys), got, want)
	}
}

func TestHeightMinimality(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for range 30 {
		n := 40 + rng.Intn(3000)
		seen := map[string]bool{}
		var keys []string
		for len(keys) < n {
			l := rng.Intn(9) + 1
			k := make([]byte, l)
			for j := range k {
				k[j] = "abcdefgh"[rng.Intn(1+rng.Intn(8))]
			}
			if !seen[string(k)] {
				seen[string(k)] = true
				keys = append(keys, string(k))
			}
		}
		rng.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
		checkMinHeight(t, keys)
	}

	// Dense integers, a paper-highlighted case.
	var dense []string
	for i := range 5000 {
		dense = append(dense, fmt.Sprintf("%08d", i))
	}
	checkMinHeight(t, dense)
}

func TestHeightMinimalityWords(t *testing.T) {
	if testing.Short() {
		t.Skip("words dataset check is slow")
	}
	words := loadTestFile("test/assets/words.txt")
	tr := newTree()
	sorted := make([][]byte, 0, len(words))
	for _, w := range words {
		tr.Insert(w, w)
		sorted = append(sorted, escapeKey(w))
	}
	slices.SortFunc(sorted, bytes.Compare)
	level, _ := smhp(buildRefTrie(sorted), maxFanout-1)
	if got, want := hdr(tr.root).height, uint8(level+1); got != want {
		t.Fatalf("words: HOT height %d != minimum height %d", got, want)
	}
}
