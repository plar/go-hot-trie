A Height Optimized Trie Implementation in Go
====

[![CI](https://github.com/plar/go-hot-trie/actions/workflows/ci.yml/badge.svg)](https://github.com/plar/go-hot-trie/actions/workflows/ci.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/plar/go-hot-trie.svg)](https://pkg.go.dev/github.com/plar/go-hot-trie)

This library provides a Go implementation of HOT, the Height Optimized Trie [1][2].
It shares the `Tree` interface with [go-adaptive-radix-tree](https://github.com/plar/go-adaptive-radix-tree),
so most code can swap between the two with an import change. The node
`Kind` constants differ: HOT reports Node8/Node16/Node32.

HOT packs a binary Patricia trie into compound nodes with a fixed maximum
fanout of 32. The span of each node adapts to the data. Insertion produces
a provably minimal tree height and the same structure for every insertion
order; deletion keeps the height close to minimal through node merges. A
node lookup compares all sparse partial keys of a node in parallel with
AVX2 instructions.

Features:
* Read-optimized: beats ART on lookups, ordered scans and memory (see benchmarks below)
* Keys are sorted **lexicographically** by byte value: ordered iteration and prefix lookups
* Minimum / Maximum value lookups
* Ordered, reverse and prefix-based iteration
* Any byte array is a valid key, including null bytes and keys that are prefixes of other keys

# Usage

```go
package main

import (
	"fmt"

	hot "github.com/plar/go-hot-trie"
)

func main() {
	tree := hot.New()

	tree.Insert(hot.Key("apple"), "A sweet red fruit")
	tree.Insert(hot.Key("banana"), "A long yellow fruit")
	tree.Insert(hot.Key("cherry"), "A small red fruit")

	if value, found := tree.Search(hot.Key("banana")); found {
		fmt.Println("Found:", value)
	}

	// Ascending order
	tree.ForEach(func(node hot.Node) bool {
		fmt.Printf("Key: %s, Value: %s\n", node.Key(), node.Value())
		return true
	})

	// Keys with a prefix
	tree.ForEachPrefix(hot.Key("c"), func(node hot.Node) bool {
		fmt.Printf("Key: %s, Value: %s\n", node.Key(), node.Value())
		return true
	})
}

// Output:
// Found: A long yellow fruit
// Key: apple, Value: A sweet red fruit
// Key: banana, Value: A long yellow fruit
// Key: cherry, Value: A small red fruit
// Key: cherry, Value: A small red fruit
```

Keys are stored by reference. Do not modify a key slice after inserting it.
The tree is not safe for concurrent use, same as go-adaptive-radix-tree.

# SIMD

The search kernels use Go's experimental [`simd/archsimd`](https://pkg.go.dev/simd)
package. Enable them with:

```sh
GOEXPERIMENT=simd go build ./...
```

Go 1.26 and 1.27 both work. Without the experiment, or on other
architectures, the library falls back to scalar kernels with a BMI2 `PEXT`
fast path on amd64 and lands within about 15% of SIMD search performance.

# Performance

Benchmarks against [go-adaptive-radix-tree/v2](https://github.com/plar/go-adaptive-radix-tree)
on the same datasets that library ships with:

- "Words": 235,886 english words
- "UUIDs": 100,000 uuids
- "HSK": 4,995 words [4]

Times are per full-dataset pass (i9-13950HX, `GOEXPERIMENT=simd`, average of
3 × 2s runs, shuffled search probes).

| Benchmark        | ART      | HOT      | HOT vs ART       |
|------------------|----------|----------|------------------|
| Search / Words   | 93.5 ms  | 74.7 ms  | **1.25× faster** |
| Search / UUIDs   | 26.8 ms  | 13.4 ms  | **2.0× faster**  |
| Search / HSK     | 384 µs   | 352 µs   | **1.1× faster**  |
| ForEach / Words  | 15.5 ms  | 2.1 ms   | **7.5× faster**  |
| ForEach / UUIDs  | 5.2 ms   | 0.5 ms   | **10.7× faster** |
| ForEach / HSK    | 496 µs   | 18 µs    | **28× faster**   |
| Insert / Words   | 60.1 ms  | 131.4 ms | 2.2× slower      |
| Insert / UUIDs   | 30.5 ms  | 47.7 ms  | 1.6× slower      |
| Delete / Words   | 25.7 ms  | 84.1 ms  | 3.3× slower      |
| Delete / UUIDs   | 16.7 ms  | 28.8 ms  | 1.7× slower      |

Retained memory after building the tree (key bytes excluded):

| Dataset | ART                 | HOT                | HOT vs ART       |
|---------|---------------------|--------------------|------------------|
| Words   | 35.2 MB (149 B/key) | 19.2 MB (81 B/key) | **1.8× smaller** |
| UUIDs   | 17.2 MB (172 B/key) | 7.1 MB (71 B/key)  | **2.4× smaller** |
| HSK     | 0.7 MB (137 B/key)  | 0.4 MB (72 B/key)  | **1.9× smaller** |

HOT trades update speed for lookups, scans and memory, as reported in the
paper. Use it for read-mostly indexes, sparse keys such as UUIDs and hashes,
and scan-heavy workloads; stay with ART for write-heavy ones.
[benchmark/README.md](benchmark/README.md) has the methodology and full
numbers. Reproduce with:

```sh
cd benchmark
GOEXPERIMENT=simd go test -bench . -benchmem
```

# Design

[docs/DESIGN.md](docs/DESIGN.md) describes the node layout, the insertion
and deletion cases from the paper, the key encoding, and where the
implementation deviates from the paper and why. The test suite checks inserted trees for
height minimality against the Kovács-Kis minimum-height partitioning and
for independence from insertion order.

# References

[1] [HOT: A Height Optimized Trie Index for Main-Memory Database Systems](https://15721.courses.cs.cmu.edu/spring2019/papers/08-oltpindexes2/p521-binna.pdf) (SIGMOD 2018)

[2] [Height Optimized Tries](https://evazangerle.at/publication/binna-tods-2022/binna-tods-2022.pdf) (ACM TODS 47(1), 2022, extended version)

[3] [go-adaptive-radix-tree](https://github.com/plar/go-adaptive-radix-tree)

[4] [HSK Words](http://hskhsk.pythonanywhere.com/hskwords). HSK(Hanyu Shuiping Kaoshi) - Standardized test of Standard Mandarin Chinese proficiency.
