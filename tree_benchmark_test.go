package hot

// Benchmarks for the tree implementation, on the datasets used by
// go-adaptive-radix-tree. Structure assertions live in the tests
// (tree_traversal_test.go); benchmarks only measure.

import "testing"

func benchmarkInsert(b *testing.B, path string) {
	words := loadTestFile(path)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		tree := New()
		for _, w := range words {
			tree.Insert(w, w)
		}
	}
}

func benchmarkSearch(b *testing.B, path string) {
	tree, words := treeWithData(path)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for _, w := range words {
			tree.Search(w)
		}
	}
}

func benchmarkIterator(b *testing.B, path string) {
	tree, _ := treeWithData(path)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		it := tree.Iterator(TraverseAll)
		for it.HasNext() {
			if _, err := it.Next(); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func benchmarkForEach(b *testing.B, path string) {
	tree, _ := treeWithData(path)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		cnt := 0
		tree.ForEach(func(Node) bool { cnt++; return true }, TraverseAll)
		if cnt == 0 {
			b.Fatal("empty traversal")
		}
	}
}

func benchmarkDelete(b *testing.B, path string) {
	words := loadTestFile(path)
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		tree := New()
		for _, w := range words {
			tree.Insert(w, w)
		}
		b.StartTimer()
		for _, w := range words {
			tree.Delete(w)
		}
	}
}

func BenchmarkWordsTreeInsert(b *testing.B)   { benchmarkInsert(b, "test/assets/words.txt") }
func BenchmarkWordsTreeSearch(b *testing.B)   { benchmarkSearch(b, "test/assets/words.txt") }
func BenchmarkWordsTreeIterator(b *testing.B) { benchmarkIterator(b, "test/assets/words.txt") }
func BenchmarkWordsTreeForEach(b *testing.B)  { benchmarkForEach(b, "test/assets/words.txt") }
func BenchmarkWordsTreeDelete(b *testing.B)   { benchmarkDelete(b, "test/assets/words.txt") }

func BenchmarkUUIDsTreeInsert(b *testing.B)   { benchmarkInsert(b, "test/assets/uuid.txt") }
func BenchmarkUUIDsTreeSearch(b *testing.B)   { benchmarkSearch(b, "test/assets/uuid.txt") }
func BenchmarkUUIDsTreeIterator(b *testing.B) { benchmarkIterator(b, "test/assets/uuid.txt") }
func BenchmarkUUIDsTreeForEach(b *testing.B)  { benchmarkForEach(b, "test/assets/uuid.txt") }
func BenchmarkUUIDsTreeDelete(b *testing.B)   { benchmarkDelete(b, "test/assets/uuid.txt") }

func BenchmarkHSKTreeInsert(b *testing.B)   { benchmarkInsert(b, "test/assets/hsk_words.txt") }
func BenchmarkHSKTreeSearch(b *testing.B)   { benchmarkSearch(b, "test/assets/hsk_words.txt") }
func BenchmarkHSKTreeIterator(b *testing.B) { benchmarkIterator(b, "test/assets/hsk_words.txt") }
func BenchmarkHSKTreeForEach(b *testing.B)  { benchmarkForEach(b, "test/assets/hsk_words.txt") }
