// Package benchmark compares HOT (github.com/plar/go-hot-trie) against ART
// (github.com/plar/go-adaptive-radix-tree) on the datasets bundled with both
// libraries. Run with GOEXPERIMENT=simd to enable HOT's SIMD kernels:
//
//	GOEXPERIMENT=simd go test -bench . -benchmem
package benchmark

import (
	"bufio"
	"math/rand"
	"os"
	"testing"

	art "github.com/plar/go-adaptive-radix-tree/v2"
	hot "github.com/plar/go-hot-trie"
)

func loadTestFile(path string) [][]byte {
	file, err := os.Open(path)
	if err != nil {
		panic("Couldn't open " + path)
	}
	defer func() { _ = file.Close() }()

	var words [][]byte
	reader := bufio.NewReader(file)
	for {
		if line, err := reader.ReadBytes(byte('\n')); err != nil {
			break
		} else if len(line) > 0 {
			words = append(words, line[:len(line)-1])
		}
	}
	return words
}

var datasets = []struct {
	name string
	path string
}{
	{"Words", "../test/assets/words.txt"},
	{"UUIDs", "../test/assets/uuid.txt"},
	{"HSK", "../test/assets/hsk_words.txt"},
}

func shuffled(words [][]byte) [][]byte {
	out := make([][]byte, len(words))
	copy(out, words)
	rng := rand.New(rand.NewSource(20260805))
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func BenchmarkTreeInsert(b *testing.B) {
	for _, ds := range datasets {
		words := loadTestFile(ds.path)
		b.Run(ds.name+"/ART", func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				tree := art.New()
				for _, w := range words {
					tree.Insert(w, w)
				}
			}
		})
		b.Run(ds.name+"/HOT", func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				tree := hot.New()
				for _, w := range words {
					tree.Insert(w, w)
				}
			}
		})
	}
}

func BenchmarkTreeSearch(b *testing.B) {
	for _, ds := range datasets {
		words := loadTestFile(ds.path)
		probe := shuffled(words)

		at := art.New()
		ht := hot.New()
		for _, w := range words {
			at.Insert(w, w)
			ht.Insert(w, w)
		}

		b.Run(ds.name+"/ART", func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				for _, w := range probe {
					at.Search(w)
				}
			}
		})
		b.Run(ds.name+"/HOT", func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				for _, w := range probe {
					ht.Search(w)
				}
			}
		})
	}
}

func BenchmarkTreeDelete(b *testing.B) {
	for _, ds := range datasets {
		words := loadTestFile(ds.path)
		b.Run(ds.name+"/ART", func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				tree := art.New()
				for _, w := range words {
					tree.Insert(w, w)
				}
				b.StartTimer()
				for _, w := range words {
					tree.Delete(w)
				}
			}
		})
		b.Run(ds.name+"/HOT", func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				tree := hot.New()
				for _, w := range words {
					tree.Insert(w, w)
				}
				b.StartTimer()
				for _, w := range words {
					tree.Delete(w)
				}
			}
		})
	}
}

func BenchmarkTreeForEach(b *testing.B) {
	for _, ds := range datasets {
		words := loadTestFile(ds.path)

		at := art.New()
		ht := hot.New()
		for _, w := range words {
			at.Insert(w, w)
			ht.Insert(w, w)
		}

		b.Run(ds.name+"/ART", func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				cnt := 0
				at.ForEach(func(art.Node) bool { cnt++; return true })
			}
		})
		b.Run(ds.name+"/HOT", func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				cnt := 0
				ht.ForEach(func(hot.Node) bool { cnt++; return true })
			}
		})
	}
}
