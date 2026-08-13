# HOT vs ART — full comparison report

- **Libraries**: `github.com/plar/go-hot-trie` (this repo) vs `github.com/plar/go-adaptive-radix-tree/v2 v2.0.4`
- **Machine**: Intel i9-13950HX (AVX2, no AVX-512), Linux, Go 1.26.5
- **Method**: average of 3 × 2s runs per benchmark; searches probe every key in shuffled
  order; Insert builds the whole tree; Delete removes every key in file order (tree
  rebuild excluded via StopTimer); ForEach walks all leaves with a counting callback.
- **Datasets**: `words` (235,886 English words), `uuid` (100,000 UUIDs),
  `hsk_words` (4,995 Chinese words, UTF-8).

## 1. Throughput — SIMD build (`GOEXPERIMENT=simd`, recommended)

Time per full-dataset pass; per-key time in parentheses.

| Operation | Dataset | ART | HOT | HOT vs ART |
|-----------|---------|-----|-----|------------|
| **Search**  | Words | 93.5 ms (396 ns) | 74.7 ms (317 ns) | **1.25× faster** |
|             | UUIDs | 26.8 ms (268 ns) | 13.4 ms (134 ns) | **2.00× faster** |
|             | HSK   | 384 µs (77 ns)   | 352 µs (70 ns)   | **1.09× faster** |
| **ForEach** | Words | 15.5 ms (66 ns)  | 2.06 ms (8.7 ns) | **7.5× faster** |
|             | UUIDs | 5.16 ms (52 ns)  | 0.48 ms (4.8 ns) | **10.7× faster** |
|             | HSK   | 496 µs (99 ns)   | 17.7 µs (3.5 ns) | **28× faster** |
| **Insert**  | Words | 60.1 ms (255 ns) | 131.4 ms (557 ns) | 2.2× slower |
|             | UUIDs | 30.5 ms (305 ns) | 47.7 ms (477 ns)  | 1.6× slower |
|             | HSK   | 1.81 ms (362 ns) | 3.31 ms (663 ns)  | 1.8× slower |
| **Delete**  | Words | 25.7 ms (109 ns) | 100.5 ms (426 ns) | 3.9× slower |
|             | UUIDs | 16.5 ms (165 ns) | 33.6 ms (336 ns)  | 2.0× slower |
|             | HSK   | 0.98 ms (195 ns) | 2.29 ms (458 ns)  | 2.3× slower |

## 2. Throughput — scalar build (no GOEXPERIMENT; BMI2 PEXT assembly active)

| Operation | Dataset | ART | HOT | HOT vs ART |
|-----------|---------|-----|-----|------------|
| **Search**  | Words | 93.3 ms | 77.9 ms | **1.20× faster** |
|             | UUIDs | 26.6 ms | 15.5 ms | **1.71× faster** |
|             | HSK   | 376 µs  | 386 µs  | 1.03× slower |
| **ForEach** | Words | 15.9 ms | 2.06 ms | **7.7× faster** |
|             | UUIDs | 5.22 ms | 0.47 ms | **11.1× faster** |
|             | HSK   | 469 µs  | 18.2 µs | **26× faster** |

The scalar build loses only ~4–15% of the SIMD build's search speed thanks to the
hardware PEXT path; HSK is the one workload where scalar HOT ties/slightly trails ART.

## 3. Memory — retained heap after building the tree

Live heap (`runtime.MemStats` after GC), key bytes excluded (shared by both libraries).

| Dataset | ART | HOT | HOT vs ART |
|---------|-----|-----|------------|
| Words | 35.19 MB (149.2 B/key) | 19.21 MB (81.4 B/key) | **1.83× smaller** |
| UUIDs | 17.17 MB (171.7 B/key) | 7.09 MB (70.9 B/key)  | **2.42× smaller** |
| HSK   | 0.68 MB (136.9 B/key)  | 0.36 MB (71.6 B/key)  | **1.91× smaller** |

## 4. Tree structure

| Metric | Dataset | ART | HOT |
|--------|---------|-----|-----|
| Inner nodes | Words | 124,256 (113,419 Node4 + 10,433 Node16 + 403 Node48 + 1 Node256) | **15,138** (5,012 Node8 + 6,364 Node16 + 3,762 Node32) |
|             | UUIDs | 37,408 (32,288 Node4 + 5,120 Node16) | **4,551** (931 Node8 + 3,620 Node16) |
|             | HSK   | 1,931 (1,630 Node4 + 276 Node16 + 21 Node48 + 4 Node256) | **267** (65 Node8 + 202 Node16) |
| Tree height / avg leaf depth | Words | — | 5 / 4.96 |
|             | UUIDs | — | 4 / 4.00 |
|             | HSK   | — | 3 / 3.00 |
| Avg node fanout | Words | 1.9 | **16.6** |
|             | UUIDs | 2.7 | **23.0** |
|             | HSK   | 2.6 | **19.7** |

(ART node counts from its own test-suite statistics; ART average fanout =
leaves ÷ inner nodes. HOT heights are provably minimal — verified against the
Kovács–Kis minimum-height-partitioning reference in `height_test.go`.)

## 5. Summary

HOT delivers what the paper promises: it is a **read-optimized, space-optimized**
index. Against ART on the same datasets it is:

- **faster on every point-lookup workload** (up to 2× on sparse keys like UUIDs),
- **an order of magnitude faster on ordered scans** (dense, pointer-contiguous
  compound nodes; zero allocations),
- **~2× smaller in resident memory** (8× fewer inner nodes at ~8× higher fanout),
- **1.6–3.9× slower on updates**, the documented trade-off: every insert/delete
  maintains linearized sparse-partial-key arrays inside 32-entry nodes, where ART
  only touches a small node — the same profile reported in the paper's own
  evaluation for string workloads.

Reproduce with:

```sh
cd benchmark
GOEXPERIMENT=simd go test -bench . -benchmem   # SIMD build
go test -bench . -benchmem                     # scalar build
```
