//go:build !race

package hot

import "testing"

// TestSearchAllocs pins the zero-allocation contract of the search path for
// NUL-free keys (the race detector inflates allocation counts, hence the
// build tag).
func TestSearchAllocs(t *testing.T) {
	tr := newTree()
	words := loadTestFile("test/assets/words.txt")[:5000]
	for _, w := range words {
		tr.Insert(w, w)
	}
	if avg := testing.AllocsPerRun(200, func() {
		for _, w := range words[:50] {
			if _, found := tr.Search(w); !found {
				t.Fatal("key missing")
			}
		}
	}); avg != 0 {
		t.Fatalf("Search allocates: %.1f allocs per 50 lookups", avg)
	}

	if avg := testing.AllocsPerRun(50, func() {
		n := 0
		tr.ForEach(func(Node) bool { n++; return true })
		if n != len(words) {
			t.Fatal("short iteration")
		}
	}); avg != 0 {
		t.Fatalf("ForEach allocates: %.1f allocs per full scan", avg)
	}
}
