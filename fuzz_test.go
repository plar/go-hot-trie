package hot

import (
	"bytes"
	"slices"
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
func FuzzTreeOps(f *testing.F) {
	f.Add([]byte("\x00\x03abc\x01\x03abc\x02\x03abc"))
	f.Add([]byte("\x00\x01a\x00\x02ab\x00\x00\x00\x01\x00\x01\x01\x02ab"))
	// Delete of an absent NUL-containing key (escaped retry that misses).
	f.Add([]byte("\x00\x02a\x00\x01\x03a\x00b"))
	// Enough distinct single-byte inserts to split the root...
	split := []byte{}
	for i := range byte(40) {
		split = append(split, 0x00, 0x00, i)
	}
	f.Add(split)
	// ...and a copy that deletes most of them again (merges, pull-downs).
	churn := slices.Clone(split)
	for i := range byte(35) {
		churn = append(churn, 0x01, 0x00, i)
	}
	f.Add(churn)
	// Long keys sharing a long prefix (gather/scatter specs, deep paths).
	long := []byte{}
	for i := range byte(6) {
		long = append(long, 0x00, 0x80|16, 'p', 'r', 'e', 'f', 'i', 'x', '-', 'p', 'r', 'e', 'f', 'i', 'x', '-', 'k', i)
	}
	f.Add(long)

	f.Fuzz(func(t *testing.T, data []byte) {
		tr := newTree()
		ref := map[string]int{}
		ctr := 0

		var itSnap []string
		var itPos int
		var it Iterator
		itValid := false

		ops := 0
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
				if it == nil {
					itSnap = sortedKeys(ref)
					itPos = 0
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
				case itPos >= len(itSnap):
					if err != ErrNoMoreNodes {
						t.Fatalf("exhausted iterator: err=%v", err)
					}
					it = nil
				default:
					if err != nil || string(n.Key()) != itSnap[itPos] {
						t.Fatalf("iterator at %d: key=%v err=%v want %q", itPos, n, err, itSnap[itPos])
					}
					itPos++
				}
			}
			if ops%64 == 0 {
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
		i := 0
		tr.ForEach(func(n Node) bool {
			if i >= len(want) || string(n.Key()) != want[i] {
				t.Fatalf("iteration mismatch at %d", i)
			}
			i++
			return true
		})
		if i != len(want) {
			t.Fatalf("iterated %d of %d", i, len(want))
		}
		i = len(want)
		tr.ForEach(func(n Node) bool {
			i--
			if i < 0 || string(n.Key()) != want[i] {
				t.Fatalf("reverse iteration mismatch at %d", i)
			}
			return true
		}, TraverseReverse)
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
	out := make([]string, 0, len(ref))
	for k := range ref {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func checkPrefixScan(t *testing.T, tr *tree, ref map[string]int, prefix string) {
	t.Helper()
	var want []string
	for k := range ref {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			want = append(want, k)
		}
	}
	slices.Sort(want)
	i := 0
	tr.ForEachPrefix(Key(prefix), func(n Node) bool {
		if i >= len(want) || string(n.Key()) != want[i] {
			t.Fatalf("prefix %q mismatch at %d: got %q", prefix, i, n.Key())
		}
		i++
		return true
	})
	if i != len(want) {
		t.Fatalf("prefix %q: got %d of %d", prefix, i, len(want))
	}
	i = len(want)
	tr.ForEachPrefix(Key(prefix), func(n Node) bool {
		i--
		if i < 0 || string(n.Key()) != want[i] {
			t.Fatalf("reverse prefix %q mismatch at %d", prefix, i)
		}
		return true
	}, TraverseReverse)
	if i != 0 {
		t.Fatalf("reverse prefix %q: %d not visited", prefix, i)
	}
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
		shape := func(order [][]byte) string {
			tr := newTree()
			for _, k := range order {
				tr.Insert(k, 0)
			}
			var sb bytes.Buffer
			treeShape(tr.root, &sb)
			return sb.String()
		}
		streamOrder := shape(keys)
		sorted := slices.Clone(keys)
		slices.SortFunc(sorted, bytes.Compare)
		if sortedOrder := shape(sorted); streamOrder != sortedOrder {
			t.Fatalf("structure depends on insertion order:\n%s\nvs\n%s", streamOrder, sortedOrder)
		}
	})
}
