package hot

import (
	"bytes"
	"fmt"
	"math/rand"
	"slices"
	"testing"
	"unsafe"
)

// checkInvariants walks the tree verifying the HOT structural invariants:
// fanout bounds, strictly ascending sparse partial keys, truthful sparse
// bits vs. the actual leaf keys, and consistent stored heights.
func checkInvariants(t *testing.T, tr *tree) {
	t.Helper()
	if tr.root == nil {
		return
	}
	n := checkNode(t, tr.root)
	if n != tr.size {
		t.Fatalf("leaf count %d != size %d", n, tr.size)
	}
	var prev Key
	first := true
	tr.ForEach(func(node Node) bool {
		if !first && bytes.Compare(prev, node.Key()) >= 0 {
			t.Fatalf("keys out of order: %q >= %q", prev, node.Key())
		}
		first = false
		prev = node.Key()
		return true
	})
}

func checkNode(t *testing.T, p unsafe.Pointer) int {
	t.Helper()
	if isLeaf(p) {
		return 1
	}
	h := hdr(p)
	if h.num < 2 || h.num > maxFanout {
		t.Fatalf("node with %d entries", h.num)
	}
	var b nb
	load(p, &b)
	if b.np < 1 || b.np > int(h.num)-1 {
		// The builder reclaims dead positions on load, so after load np
		// reflects live BiNodes only and must fit the fanout bound.
		t.Fatalf("node with %d entries has %d live positions", h.num, b.np)
	}
	// positions strictly ascending
	for i := 1; i < b.np; i++ {
		if b.positions[i-1] >= b.positions[i] {
			t.Fatalf("positions not ascending: %v", b.positions[:b.np])
		}
	}
	// sparse keys strictly ascending
	for i := 1; i < b.ne; i++ {
		if b.sparse[i-1] >= b.sparse[i] {
			t.Fatalf("sparse keys not ascending at %d: %08x %08x", i, b.sparse[i-1], b.sparse[i])
		}
	}
	// every set sparse bit must be truthful: the corresponding bit of every
	// key below the entry is 1 at that position
	for i := 0; i < b.ne; i++ {
		mk := minimumLeaf(b.children[i]).key
		ek := escapeKey(mk)
		s := b.sparse[i]
		for r := 0; r < b.np; r++ {
			if s&rankBit(r) != 0 && keyBitAt(ek, b.positions[r]) != 1 {
				t.Fatalf("sparse bit rank %d (pos %d) set but key bit is 0 for key %q", r, b.positions[r], mk)
			}
		}
	}
	if ch := computeHeight(p); ch != h.height {
		t.Fatalf("stored height %d != computed %d", h.height, ch)
	}
	total := 0
	for i := 0; i < int(h.num); i++ {
		total += checkNode(t, childAt(p, i))
	}
	return total
}

// treeShape renders the structure (positions, sparse keys, leaf keys) for
// determinism comparison.
func treeShape(p unsafe.Pointer, sb *bytes.Buffer) {
	if p == nil {
		return
	}
	if isLeaf(p) {
		fmt.Fprintf(sb, "L%q", (*leaf)(p).key)
		return
	}
	var b nb
	load(p, &b)
	fmt.Fprintf(sb, "N(h%d,p%v,s%v)[", hdr(p).height, b.positions[:b.np], b.sparse[:b.ne])
	for i := 0; i < b.ne; i++ {
		treeShape(b.children[i], sb)
		sb.WriteByte(',')
	}
	sb.WriteByte(']')
}

func TestSmokeSequential(t *testing.T) {
	tr := newTree()
	const n = 1000
	for i := range n {
		k := Key(fmt.Sprintf("key-%04d", i))
		old, upd := tr.Insert(k, i)
		if old != nil || upd {
			t.Fatalf("unexpected update for %q", k)
		}
	}
	checkInvariants(t, tr)
	for i := range n {
		k := Key(fmt.Sprintf("key-%04d", i))
		v, found := tr.Search(k)
		if !found || v != i {
			t.Fatalf("search %q: got %v %v", k, v, found)
		}
	}
	if tr.Size() != n {
		t.Fatalf("size %d", tr.Size())
	}
	for i := range n {
		k := Key(fmt.Sprintf("key-%04d", i))
		v, del := tr.Delete(k)
		if !del || v != i {
			t.Fatalf("delete %q: got %v %v", k, v, del)
		}
		if i%97 == 0 {
			checkInvariants(t, tr)
		}
	}
	if tr.Size() != 0 || tr.root != nil {
		t.Fatalf("tree not empty after deleting all")
	}
}

func TestSmokeRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	tr := newTree()
	ref := map[string]int{}
	var keys []string
	for i := range 5000 {
		l := rng.Intn(12) + 1
		k := make([]byte, l)
		for j := range k {
			k[j] = byte(rng.Intn(256)) // includes NUL bytes
		}
		if _, ok := ref[string(k)]; !ok {
			keys = append(keys, string(k))
		}
		ref[string(k)] = i
		tr.Insert(Key(k), i)
	}
	if tr.Size() != len(ref) {
		t.Fatalf("size %d != %d", tr.Size(), len(ref))
	}
	checkInvariants(t, tr)
	for k, v := range ref {
		got, found := tr.Search(Key(k))
		if !found || got != v {
			t.Fatalf("search %q: got %v %v want %v", k, got, found, v)
		}
	}
	// iteration order == sorted order
	slices.Sort(keys)
	i := 0
	tr.ForEach(func(n Node) bool {
		if string(n.Key()) != keys[i] {
			t.Fatalf("iteration order mismatch at %d: %q != %q", i, n.Key(), keys[i])
		}
		i++
		return true
	})
	if i != len(keys) {
		t.Fatalf("iterated %d of %d", i, len(keys))
	}
	// delete a random half, verify, then the rest
	perm := rng.Perm(len(keys))
	for idx, pi := range perm {
		k := keys[pi]
		v, del := tr.Delete(Key(k))
		if !del || v != ref[k] {
			t.Fatalf("delete %q: got %v %v", k, v, del)
		}
		if g, found := tr.Search(Key(k)); found {
			t.Fatalf("key %q still found after delete: %v", k, g)
		}
		if idx%512 == 0 {
			checkInvariants(t, tr)
		}
	}
	if tr.Size() != 0 || tr.root != nil {
		t.Fatal("tree not empty")
	}
}

func TestDeterminism(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	var keys []string
	seen := map[string]bool{}
	for len(keys) < 800 {
		l := rng.Intn(10) + 1
		k := make([]byte, l)
		for j := range k {
			k[j] = "abcdef01"[rng.Intn(8)]
		}
		if !seen[string(k)] {
			seen[string(k)] = true
			keys = append(keys, string(k))
		}
	}
	shape := func(order []string) string {
		tr := newTree()
		for _, k := range order {
			tr.Insert(Key(k), k)
		}
		var sb bytes.Buffer
		treeShape(tr.root, &sb)
		return sb.String()
	}
	base := shape(keys)
	for trial := range 5 {
		shuffled := append([]string(nil), keys...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		if got := shape(shuffled); got != base {
			t.Fatalf("trial %d: tree structure depends on insertion order", trial)
		}
	}
}
