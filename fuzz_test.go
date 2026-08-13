package hot

import (
	"bytes"
	"maps"
	"slices"
	"strings"
	"testing"
)

// fuzzKey derives a key from the stream: a pattern of 1..17 bytes, repeated
// up to 7 times, so short keys stay common while long keys (up to ~119
// bytes) reach the scatter extraction spec, deep trees and the path-stack
// growth paths.
func fuzzKey(pattern []byte, rep byte) []byte {
	n := int(rep)%7 + 1
	key := make([]byte, 0, len(pattern)*n)
	for range n {
		key = append(key, pattern...)
	}
	return key
}

// FuzzTreeOps drives a random operation stream (insert/delete/search plus a
// live iterator) against a map reference and the structural invariant
// checker. Values are op counters, so stale values, wrong Insert old-values
// and wrong Delete return values are all visible.
//
// Records: 1 op byte, 1 length/repeat byte, then length%17+1 pattern bytes.
// rec encodes one op-stream record, panicking if the pattern length does
// not match what the decoder will read back.
func rec(op, lenRep byte, pattern ...byte) []byte {
	if len(pattern) != int(lenRep)%17+1 {
		panic("seed pattern length does not match its length byte")
	}
	return append([]byte{op, lenRep}, pattern...)
}

func FuzzTreeOps(f *testing.F) {
	f.Add([]byte("\x00\x03abc\x01\x03abc\x02\x03abc"))
	f.Add([]byte("\x00\x01a\x00\x02ab\x00\x00\x00\x01\x00\x01\x01\x02ab"))
	// Delete of an absent NUL-containing key (escaped retry that misses).
	f.Add([]byte("\x00\x02a\x00\x01\x03a\x00b"))
	// Enough distinct single-byte inserts to split the root...
	split := []byte{}
	for i := range byte(40) {
		split = append(split, rec(0x00, 0x00, i)...)
	}
	f.Add(split)
	// ...and a copy that deletes most of them again (merges, pull-downs).
	churn := slices.Clone(split)
	for i := range byte(35) {
		churn = append(churn, rec(0x01, 0x00, i)...)
	}
	f.Add(churn)
	// Long keys sharing a long prefix (gather/scatter specs, deep paths):
	// lenRep 0x20 decodes to a 16-byte pattern repeated 5 times (80 bytes).
	long := []byte{}
	for i := range byte(6) {
		long = append(long, rec(0x00, 0x20, 'p', 'r', 'e', 'f', 'i', 'x', '-', 'p', 'r', 'e', 'f', 'i', 'x', '-', 'k', i)...)
	}
	f.Add(long)

	f.Fuzz(func(t *testing.T, data []byte) {
		tr := newTree()
		ref := map[string]int{}
		ctr := 0

		var it Iterator
		var itPrev string
		var itSeen, itCount int
		itValid, itFirst := false, true

		ops, lastCheck := 0, 0
		for len(data) >= 2 {
			op := data[0]
			kl := int(data[1])%17 + 1
			rep := data[1]
			data = data[2:]
			if len(data) < kl {
				break
			}
			key := fuzzKey(data[:kl], rep)
			data = data[kl:]
			ops++

			switch op % 4 {
			case 0: // insert
				ctr++
				old, updated := tr.Insert(key, ctr)
				prev, existed := ref[string(key)]
				if updated != existed {
					t.Fatalf("insert %q: updated=%v existed=%v", key, updated, existed)
				}
				if existed && old.(int) != prev {
					t.Fatalf("insert %q: old=%v want %d", key, old, prev)
				}
				ref[string(key)] = ctr
				itValid = false
			case 1: // delete
				v, deleted := tr.Delete(key)
				prev, existed := ref[string(key)]
				if deleted != existed {
					t.Fatalf("delete %q: deleted=%v existed=%v", key, deleted, existed)
				}
				if existed {
					if v.(int) != prev {
						t.Fatalf("delete %q: value=%v want %d", key, v, prev)
					}
					itValid = false
				}
				delete(ref, string(key))
			case 2: // search
				v, found := tr.Search(key)
				want, existed := ref[string(key)]
				if found != existed {
					t.Fatalf("search %q: found=%v existed=%v", key, found, existed)
				}
				if found && v.(int) != want {
					t.Fatalf("search %q: value=%v want %d", key, v, want)
				}
			case 3: // advance a live ordered iterator
				// Oracle without a sorted snapshot: while the iterator is
				// valid ref is frozen, so a strictly increasing sequence of
				// ref-member keys of length len(ref) is exactly sorted(ref).
				if it == nil {
					itCount = len(ref)
					itSeen, itFirst = 0, true
					it = tr.Iterator()
					itValid = true
				}
				n, err := it.Next()
				switch {
				case !itValid:
					if err != ErrConcurrentModification {
						t.Fatalf("iterator after modification: err=%v", err)
					}
					it = nil
				case itSeen >= itCount:
					if err != ErrNoMoreNodes {
						t.Fatalf("exhausted iterator: err=%v", err)
					}
					it = nil
				default:
					if err != nil {
						t.Fatalf("iterator at %d: err=%v", itSeen, err)
					}
					k := string(n.Key())
					if _, ok := ref[k]; !ok || (!itFirst && k <= itPrev) {
						t.Fatalf("iterator at %d: key %q (member=%v prev=%q)", itSeen, k, ok, itPrev)
					}
					itPrev, itFirst = k, false
					itSeen++
				}
			}
			if ops-lastCheck >= max(64, tr.Size()) {
				lastCheck = ops
				checkInvariants(t, tr)
			}
		}

		if tr.Size() != len(ref) {
			t.Fatalf("size %d != %d", tr.Size(), len(ref))
		}
		checkInvariants(t, tr)

		// Every reference key must be found with its current value; derived
		// absent keys must miss.
		for k, want := range ref {
			v, found := tr.Search(Key(k))
			if !found || v.(int) != want {
				t.Fatalf("final search %q: %v %v want %d", k, v, found, want)
			}
			absent := []byte(k + "\x00absent")
			if _, found := tr.Search(absent); found {
				t.Fatalf("absent key %q found", absent)
			}
			flipped := []byte(k)
			flipped[0] ^= 0xFF
			if _, existed := ref[string(flipped)]; !existed {
				if _, found := tr.Search(flipped); found {
					t.Fatalf("absent key %q found", flipped)
				}
			}
		}

		// Ordered iteration, reverse iteration, minimum and maximum against
		// the sorted reference.
		want := sortedKeys(ref)
		expectOrder(t, "forward", want, func(cb Callback) { tr.ForEach(cb) })
		expectOrder(t, "reverse", reversed(want), func(cb Callback) { tr.ForEach(cb, TraverseReverse) })
		if len(want) > 0 {
			if v, ok := tr.Minimum(); !ok || v.(int) != ref[want[0]] {
				t.Fatalf("minimum: %v %v", v, ok)
			}
			if v, ok := tr.Maximum(); !ok || v.(int) != ref[want[len(want)-1]] {
				t.Fatalf("maximum: %v %v", v, ok)
			}
			// Prefix scans, forward and reverse, against a filtered reference.
			for _, p := range []string{want[0], want[len(want)/2], want[len(want)-1][:1+len(want[len(want)-1])/2], "\x00", "\xff"} {
				checkPrefixScan(t, tr, ref, p)
			}
		}
	})
}

func sortedKeys(ref map[string]int) []string {
	return slices.Sorted(maps.Keys(ref))
}

// expectOrder asserts that a leaf scan yields exactly want, in order.
func expectOrder(t *testing.T, name string, want []string, scan func(Callback)) {
	t.Helper()
	i := 0
	scan(func(n Node) bool {
		if i >= len(want) || string(n.Key()) != want[i] {
			t.Fatalf("%s scan mismatch at %d: got %q", name, i, n.Key())
		}
		i++
		return true
	})
	if i != len(want) {
		t.Fatalf("%s scan: got %d of %d", name, i, len(want))
	}
}

func reversed(s []string) []string {
	out := slices.Clone(s)
	slices.Reverse(out)
	return out
}

// checkPrefixScan asserts forward and reverse ForEachPrefix against the
// prefix-filtered reference.
func checkPrefixScan(t *testing.T, tr *tree, ref map[string]int, prefix string) {
	t.Helper()
	var want []string
	for k := range ref {
		if strings.HasPrefix(k, prefix) {
			want = append(want, k)
		}
	}
	slices.Sort(want)
	expectOrder(t, "prefix "+prefix, want, func(cb Callback) { tr.ForEachPrefix(Key(prefix), cb) })
	expectOrder(t, "reverse prefix "+prefix, reversed(want), func(cb Callback) { tr.ForEachPrefix(Key(prefix), cb, TraverseReverse) })
}

// FuzzKeyEncoding checks that the escaped encoding with the virtual
// terminator is prefix-free and strictly order-preserving.
func FuzzKeyEncoding(f *testing.F) {
	f.Add([]byte("a"), []byte("ab"))
	f.Add([]byte("a\x00"), []byte("a"))
	f.Add([]byte{}, []byte{0})
	f.Add([]byte("same"), []byte("same"))
	f.Fuzz(func(t *testing.T, a, b []byte) {
		ea := append(slices.Clone(escapeKey(a)), 0x00, 0x01)
		eb := append(slices.Clone(escapeKey(b)), 0x00, 0x01)
		if got, want := bytes.Compare(ea, eb), bytes.Compare(a, b); got != want {
			t.Fatalf("order not preserved: cmp(%q,%q)=%d but cmp(enc)=%d", a, b, want, got)
		}
		if !bytes.Equal(a, b) && (bytes.HasPrefix(ea, eb) || bytes.HasPrefix(eb, ea)) {
			t.Fatalf("encoding not prefix-free for %q vs %q", a, b)
		}
		// mismatchBit must agree with the materialized encodings.
		mb := mismatchBit(escapeKey(a), escapeKey(b))
		if bytes.Equal(a, b) {
			if mb != -1 {
				t.Fatalf("equal keys with mismatch bit %d", mb)
			}
			return
		}
		i := 0
		for i < len(ea) && i < len(eb) && ea[i] == eb[i] {
			i++
		}
		if mb/8 != i {
			t.Fatalf("mismatch byte %d, encodings differ at %d", mb/8, i)
		}
	})
}

// FuzzDeterminism checks the paper's determinism property: for a fixed key
// set, every insertion order yields the same structure. Insert-only, since
// lazy dead-position reclamation makes post-delete shapes history-dependent.
func FuzzDeterminism(f *testing.F) {
	f.Add([]byte("\x03abc\x03abd\x01a\x02ab\x05abcde"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var keys [][]byte
		seen := map[string]bool{}
		for len(data) >= 1 {
			kl := int(data[0])%17 + 1
			rep := data[0]
			data = data[1:]
			if len(data) < kl {
				break
			}
			key := fuzzKey(data[:kl], rep)
			data = data[kl:]
			if !seen[string(key)] {
				seen[string(key)] = true
				keys = append(keys, key)
			}
		}
		if len(keys) < 2 {
			return
		}
		streamOrder := shapeOf(keys)
		sorted := slices.Clone(keys)
		slices.SortFunc(sorted, bytes.Compare)
		if sortedOrder := shapeOf(sorted); streamOrder != sortedOrder {
			t.Fatalf("structure depends on insertion order:\n%s\nvs\n%s", streamOrder, sortedOrder)
		}
	})
}
