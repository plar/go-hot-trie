package hot

import (
	"bytes"
	"slices"
	"testing"
)

// FuzzTreeOps drives a random operation stream (insert/delete/search)
// against a map reference and the structural invariant checker.
//
// The input is consumed as records: 1 op byte, 1 length byte, then
// length%17+1 key bytes (so NUL bytes and repeated keys occur naturally).
func FuzzTreeOps(f *testing.F) {
	f.Add([]byte("\x00\x03abc\x01\x03abc\x02\x03abc"))
	f.Add([]byte("\x00\x01a\x00\x02ab\x00\x00\x00\x01\x00\x01\x01\x02ab"))
	f.Fuzz(func(t *testing.T, data []byte) {
		tr := newTree()
		ref := map[string][]byte{}
		for len(data) >= 2 {
			op := data[0]
			kl := int(data[1])%17 + 1
			data = data[2:]
			if len(data) < kl {
				break
			}
			key := data[:kl]
			data = data[kl:]

			switch op % 3 {
			case 0: // insert
				old, updated := tr.Insert(key, string(key))
				_, existed := ref[string(key)]
				if updated != existed {
					t.Fatalf("insert %q: updated=%v existed=%v", key, updated, existed)
				}
				_ = old
				ref[string(key)] = key
			case 1: // delete
				_, deleted := tr.Delete(key)
				_, existed := ref[string(key)]
				if deleted != existed {
					t.Fatalf("delete %q: deleted=%v existed=%v", key, deleted, existed)
				}
				delete(ref, string(key))
			case 2: // search
				v, found := tr.Search(key)
				_, existed := ref[string(key)]
				if found != existed {
					t.Fatalf("search %q: found=%v existed=%v", key, found, existed)
				}
				if found && v.(string) != string(key) {
					t.Fatalf("search %q: wrong value %v", key, v)
				}
			}
		}
		if tr.Size() != len(ref) {
			t.Fatalf("size %d != %d", tr.Size(), len(ref))
		}
		checkInvariants(t, tr)
		// Iteration must yield exactly the reference keys in sorted order.
		want := make([]string, 0, len(ref))
		for k := range ref {
			want = append(want, k)
		}
		slices.Sort(want)
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
	})
}

// FuzzKeyEncoding checks that the escaped encoding with the virtual
// terminator is prefix-free and strictly order-preserving.
func FuzzKeyEncoding(f *testing.F) {
	f.Add([]byte("a"), []byte("ab"))
	f.Add([]byte("a\x00"), []byte("a"))
	f.Add([]byte{}, []byte{0})
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
