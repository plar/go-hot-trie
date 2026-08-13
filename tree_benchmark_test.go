package hot

// Ported from github.com/plar/go-adaptive-radix-tree tree_benchmark_test.go
// with structure statistics adapted to HOT's deterministic node counts.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Benchmarks for the tree implementation.
func BenchmarkWordsTreeInsert(b *testing.B) {
	words := loadTestFile("test/assets/words.txt")

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		tree := New()
		for _, w := range words {
			tree.Insert(w, w)
		}
	}
}

func BenchmarkWordsTreeSearch(b *testing.B) {
	tree, words := treeWithData("test/assets/words.txt")

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		for _, w := range words {
			tree.Search(w)
		}
	}
}

func BenchmarkWordsTreeIterator(b *testing.B) {
	tree, _ := treeWithData("test/assets/words.txt")

	b.ResetTimer()

	stats := collectStats(tree.Iterator(TraverseAll))
	assert.Equal(b, wordsStats, stats)
}

func BenchmarkWordsTreeForEach(b *testing.B) {
	tree, _ := treeWithData("test/assets/words.txt")

	b.ResetTimer()

	stats := treeStats{}
	tree.ForEach(stats.processStats, TraverseAll)
	assert.Equal(b, wordsStats, stats)

	stats = treeStats{}
	tree.ForEach(stats.processStats, TraverseLeaf)
	assert.Equal(b, treeStats{235886, 0, 0, 0}, stats)

	stats = treeStats{}
	tree.ForEach(stats.processStats, TraverseNode)
	assert.Equal(b, treeStats{0, wordsStats.node8Count, wordsStats.node16Count, wordsStats.node32Count}, stats)
}

func BenchmarkUUIDsTreeInsert(b *testing.B) {
	words := loadTestFile("test/assets/uuid.txt")

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		tree := New()
		for _, w := range words {
			tree.Insert(w, w)
		}
	}
}

func BenchmarkUUIDsTreeSearch(b *testing.B) {
	tree, words := treeWithData("test/assets/uuid.txt")

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		for _, w := range words {
			tree.Search(w)
		}
	}
}

func BenchmarkUUIDsTreeIterator(b *testing.B) {
	tree, _ := treeWithData("test/assets/uuid.txt")

	b.ResetTimer()

	stats := collectStats(tree.Iterator(TraverseAll))
	assert.Equal(b, uuidStats, stats)
}

func BenchmarkUUIDsTreeForEach(b *testing.B) {
	tree, _ := treeWithData("test/assets/uuid.txt")

	b.ResetTimer()

	stats := treeStats{}
	tree.ForEach(stats.processStats, TraverseAll)
	assert.Equal(b, uuidStats, stats)
}

func BenchmarkHSKTreeInsert(b *testing.B) {
	words := loadTestFile("test/assets/hsk_words.txt")

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		tree := New()
		for _, w := range words {
			tree.Insert(w, w)
		}
	}
}

func BenchmarkHSKTreeSearch(b *testing.B) {
	tree, words := treeWithData("test/assets/hsk_words.txt")

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		for _, w := range words {
			tree.Search(w)
		}
	}
}

func BenchmarkHSKTreeIterator(b *testing.B) {
	tree, _ := treeWithData("test/assets/hsk_words.txt")

	b.ResetTimer()

	stats := collectStats(tree.Iterator(TraverseAll))
	assert.Equal(b, hskStats, stats)
}

func BenchmarkHSKTreeForEach(b *testing.B) {
	tree, _ := treeWithData("test/assets/hsk_words.txt")

	b.ResetTimer()

	stats := treeStats{}
	tree.ForEach(stats.processStats, TraverseAll)
	assert.Equal(b, hskStats, stats)
}
