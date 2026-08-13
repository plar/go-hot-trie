package hot

import (
	"sync"
	"testing"
)

// TestAPIContractEdges pins small API contracts that the big suites never
// touch.
func TestAPIContractEdges(t *testing.T) {
	t.Run("kind strings", func(t *testing.T) {
		for k, want := range map[Kind]string{Leaf: "Leaf", Node8: "Node8", Node16: "Node16", Node32: "Node32"} {
			if got := k.String(); got != want {
				t.Fatalf("Kind(%d).String() = %q", k, got)
			}
		}
	})

	t.Run("next without hasnext", func(t *testing.T) {
		tr := newTree()
		tr.Insert(Key("a"), 1)
		tr.Insert(Key("b"), 2)
		it := tr.Iterator()
		n, err := it.Next() // no HasNext first
		if err != nil || string(n.Key()) != "a" {
			t.Fatalf("Next without HasNext: %v %v", n, err)
		}
	})

	t.Run("foreach cancel on internal node", func(t *testing.T) {
		tr := newTree()
		tr.Insert(Key("a"), 1)
		tr.Insert(Key("b"), 2)
		calls := 0
		tr.ForEach(func(n Node) bool {
			calls++
			return n.Kind() == Leaf // false on the internal root, visited first
		}, TraverseAll)
		if calls != 1 {
			t.Fatalf("expected traversal to stop at the internal root, got %d calls", calls)
		}
	})

	t.Run("traverse node with leaf root", func(t *testing.T) {
		tr := newTree()
		tr.Insert(Key("only"), 1)
		it := tr.Iterator(TraverseNode)
		if it.HasNext() {
			t.Fatal("leaf root must not be yielded under TraverseNode")
		}
		count := 0
		tr.ForEach(func(Node) bool { count++; return true }, TraverseNode)
		if count != 0 {
			t.Fatalf("ForEach TraverseNode on leaf root: %d calls", count)
		}
	})

	t.Run("empty iterator then modify", func(t *testing.T) {
		tr := newTree()
		it := tr.Iterator()
		tr.Insert(Key("a"), 1)
		if it.HasNext() {
			t.Fatal("iterator created on an empty tree has nothing to yield")
		}
		// The version check still wins over exhaustion, matching the
		// concurrent-modification contract.
		if _, err := it.Next(); err != ErrConcurrentModification {
			t.Fatalf("Next after modification: %v", err)
		}
	})

	t.Run("value update bumps version", func(t *testing.T) {
		tr := newTree()
		tr.Insert(Key("a"), 1)
		it := tr.Iterator()
		tr.Insert(Key("a"), 2) // value-only update
		if _, err := it.Next(); err != ErrConcurrentModification {
			t.Fatalf("value update must invalidate iterators: %v", err)
		}
	})

	t.Run("nil values", func(t *testing.T) {
		tr := newTree()
		if old, updated := tr.Insert(Key("k"), nil); old != nil || updated {
			t.Fatalf("insert nil: %v %v", old, updated)
		}
		if v, found := tr.Search(Key("k")); !found || v != nil {
			t.Fatalf("search nil value: %v %v", v, found)
		}
		if old, updated := tr.Insert(Key("k"), "x"); old != nil || !updated {
			t.Fatalf("update over nil: %v %v", old, updated)
		}
		if old, updated := tr.Insert(Key("k"), nil); old != "x" || !updated {
			t.Fatalf("update to nil: %v %v", old, updated)
		}
		if v, deleted := tr.Delete(Key("k")); !deleted || v != nil {
			t.Fatalf("delete nil value: %v %v", v, deleted)
		}
	})

	t.Run("empty key delete", func(t *testing.T) {
		tr := newTree()
		tr.Insert(Key(""), "empty")
		tr.Insert(Key("a"), 1)
		if v, deleted := tr.Delete(Key("")); !deleted || v != "empty" {
			t.Fatalf("delete empty key: %v %v", v, deleted)
		}
		if _, found := tr.Search(Key("")); found {
			t.Fatal("empty key still present")
		}
		if v, found := tr.Search(Key("a")); !found || v != 1 {
			t.Fatalf("sibling of empty key lost: %v %v", v, found)
		}
	})
}

// TestConcurrentReaders documents and verifies the read-only concurrency
// contract: any number of goroutines may read a tree that no goroutine is
// mutating. The race detector is the oracle.
func TestConcurrentReaders(t *testing.T) {
	tr := newTree()
	words := loadTestFile("test/assets/words.txt")[:20000]
	for _, w := range words {
		tr.Insert(w, w)
	}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := g; i < len(words); i += 8 {
				if _, found := tr.Search(words[i]); !found {
					t.Errorf("key %q not found", words[i])
					return
				}
			}
			n := 0
			tr.ForEach(func(Node) bool { n++; return n < 1000 })
			tr.ForEachPrefix(Key("an"), func(Node) bool { return true })
			if _, ok := tr.Minimum(); !ok {
				t.Error("minimum failed")
			}
			it := tr.Iterator()
			for j := 0; j < 100 && it.HasNext(); j++ {
				if _, err := it.Next(); err != nil {
					t.Errorf("iterator: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}
