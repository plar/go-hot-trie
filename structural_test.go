package hot

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

// TestDeleteUnderflowCascade splits a node, then deletes the keys one by one
// with the invariant checker (which verifies stored vs computed heights)
// after every step, driving the merge, BiNode pull-down and hoist paths and
// asserting the height shrinks back.
func TestDeleteUnderflowCascade(t *testing.T) {
	for _, order := range []string{"forward", "reverse", "inside-out"} {
		t.Run(order, func(t *testing.T) {
			tr := newTree()
			var keys []Key
			for i := range 48 {
				keys = append(keys, Key{byte(i * 5)})
			}
			for _, k := range keys {
				tr.Insert(k, string(k))
			}
			if h := hdr(tr.root).height; h < 2 {
				t.Fatalf("expected a split tree, height %d", h)
			}
			del := make([]Key, len(keys))
			copy(del, keys)
			switch order {
			case "reverse":
				for i, j := 0, len(del)-1; i < j; i, j = i+1, j-1 {
					del[i], del[j] = del[j], del[i]
				}
			case "inside-out":
				mid := len(del) / 2
				var mixed []Key
				for i := range del {
					if mid+i/2 < len(del) && i%2 == 0 {
						mixed = append(mixed, del[mid+i/2])
					} else if mid-1-i/2 >= 0 {
						mixed = append(mixed, del[mid-1-i/2])
					}
				}
				del = mixed
			}
			sawShrink := false
			maxH := hdr(tr.root).height
			for _, k := range del {
				if _, ok := tr.Delete(k); !ok {
					t.Fatalf("delete %x failed", k)
				}
				checkInvariants(t, tr)
				if tr.root != nil && !isLeaf(tr.root) && hdr(tr.root).height < maxH {
					sawShrink = true
				}
			}
			if tr.Size() != 0 || tr.root != nil {
				t.Fatal("tree not empty")
			}
			if !sawShrink {
				t.Fatal("root height never shrank during deletions")
			}
		})
	}
}

// TestLongKeysDeepTree drives long keys (up to 600 bytes) sharing long
// prefixes: discriminative positions spread over hundreds of bytes (scatter
// specs), the tree grows deeper than the path-stack backing arrays, and
// deleting back down shrinks ancestor heights.
func TestLongKeysDeepTree(t *testing.T) {
	tr := newTree()
	var keys []Key
	// A prefix chain: "a", "aa", ... up to 600 bytes.
	for i := 1; i <= 600; i += 7 {
		keys = append(keys, Key(strings.Repeat("a", i)))
	}
	// 300-byte keys differing only in the last byte or one middle bit.
	base := strings.Repeat("x", 300)
	for i := range 40 {
		k := []byte(base)
		k[280+i%20] ^= byte(1 << (i % 8))
		keys = append(keys, k)
	}
	for i, k := range keys {
		tr.Insert(k, i)
		if i%16 == 0 {
			checkInvariants(t, tr)
		}
	}
	checkInvariants(t, tr)
	for i, k := range keys {
		if v, found := tr.Search(k); !found || v.(int) != i {
			t.Fatalf("search key %d (len %d): %v %v", i, len(k), v, found)
		}
	}
	// Ordered iteration count.
	n := 0
	tr.ForEach(func(Node) bool { n++; return true })
	if n != len(keys) {
		t.Fatalf("iterated %d of %d", n, len(keys))
	}
	// Delete in insertion order with invariants; ancestor heights must shrink.
	for i, k := range keys {
		if _, ok := tr.Delete(k); !ok {
			t.Fatalf("delete key %d failed", i)
		}
		if i%8 == 0 {
			checkInvariants(t, tr)
		}
	}
	if tr.Size() != 0 || tr.root != nil {
		t.Fatal("tree not empty")
	}
}

// TestChurn interleaves inserts, deletes and searches over a small key pool
// so nodes see delete-then-reinsert traffic: dead discriminative positions
// get revived by ipInsert and reclaimed by builder rebuilds.
func TestChurn(t *testing.T) {
	rng := rand.New(rand.NewSource(77))
	pool := make([]Key, 300)
	for i := range pool {
		l := 1 + rng.Intn(6)
		k := make([]byte, l)
		for j := range k {
			k[j] = byte(rng.Intn(8) * 0x20) // few distinct bytes: shared nodes
		}
		k[0] = byte(i % 16 * 0x11)
		pool[i] = k
	}
	ref := map[string]int{}
	tr := newTree()
	for op := range 30000 {
		k := pool[rng.Intn(len(pool))]
		switch rng.Intn(3) {
		case 0:
			old, updated := tr.Insert(k, op)
			prev, existed := ref[string(k)]
			if updated != existed || (existed && old.(int) != prev) {
				t.Fatalf("op %d insert %x: old=%v updated=%v want %d %v", op, k, old, updated, prev, existed)
			}
			ref[string(k)] = op
		case 1:
			v, deleted := tr.Delete(k)
			prev, existed := ref[string(k)]
			if deleted != existed || (existed && v.(int) != prev) {
				t.Fatalf("op %d delete %x: v=%v deleted=%v want %d %v", op, k, v, deleted, prev, existed)
			}
			delete(ref, string(k))
		case 2:
			v, found := tr.Search(k)
			want, existed := ref[string(k)]
			if found != existed || (found && v.(int) != want) {
				t.Fatalf("op %d search %x: v=%v found=%v want %d %v", op, k, v, found, want, existed)
			}
		}
		if op%128 == 0 {
			checkInvariants(t, tr)
		}
	}
	checkInvariants(t, tr)
	for k, want := range ref {
		if v, found := tr.Search(Key(k)); !found || v.(int) != want {
			t.Fatalf("final search %x: %v %v want %d", k, v, found, want)
		}
	}
}

// TestDeleteAbsentNulKey covers the escaped-retry path that still misses.
func TestDeleteAbsentNulKey(t *testing.T) {
	tr := newTree()
	tr.Insert(Key("ab\x00"), 1)
	tr.Insert(Key("ab"), 2)
	for _, k := range []Key{Key("ab\x00x"), Key("a\x00"), Key("\x00")} {
		if v, deleted := tr.Delete(k); deleted {
			t.Fatalf("deleted absent key %q: %v", k, v)
		}
	}
	if tr.Size() != 2 {
		t.Fatalf("size %d", tr.Size())
	}
	if v, found := tr.Search(Key("ab\x00")); !found || v.(int) != 1 {
		t.Fatalf("nul key lost: %v %v", v, found)
	}
}

// TestPrefixScanNulPrefix covers ForEachPrefix with NUL-containing prefixes.
func TestPrefixScanNulPrefix(t *testing.T) {
	tr := newTree()
	keys := []string{"a", "a\x00", "a\x00b", "a\x00\x00", "a\x01", "b\x00"}
	for i, k := range keys {
		tr.Insert(Key(k), i)
	}
	for _, prefix := range []string{"a\x00", "\x00", "a", "b\x00", "a\x00\x00"} {
		var got []string
		tr.ForEachPrefix(Key(prefix), func(n Node) bool {
			got = append(got, string(n.Key()))
			return true
		})
		var want []string
		for _, k := range keys {
			if strings.HasPrefix(k, prefix) {
				want = append(want, k)
			}
		}
		slices.Sort(want)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("prefix %q: got %q want %q", prefix, got, want)
		}
	}
}

// TestSmokeRandomSurvivors deletes half a random tree and then verifies
// every survivor is still reachable with its value: merge and pull-down
// rebuilds must preserve full search semantics, not just structure.
func TestSmokeRandomSurvivors(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	tr := newTree()
	ref := map[string]int{}
	for i := range 8000 {
		l := rng.Intn(12) + 1
		k := make([]byte, l)
		rng.Read(k)
		ref[string(k)] = i
		tr.Insert(Key(k), i)
	}
	keys := sortedKeys(ref)
	rng.Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	half := len(keys) / 2
	for _, k := range keys[:half] {
		if _, ok := tr.Delete(Key(k)); !ok {
			t.Fatalf("delete %x failed", k)
		}
		delete(ref, k)
	}
	checkInvariants(t, tr)
	for _, k := range keys[half:] {
		if v, found := tr.Search(Key(k)); !found || v.(int) != ref[k] {
			t.Fatalf("survivor %x: %v %v want %d", k, v, found, ref[k])
		}
	}
}
