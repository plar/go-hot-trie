# go-hot-trie — design

Design spec for the initial implementation (2026-08-05), kept as the record
of the architecture and of the deliberate deviations from the paper.

## Goal

Implement HOT (Binna et al., "HOT: A Height Optimized Trie Index for Main-Memory Database
Systems", SIGMOD 2018 / TODS 2022) as a Go library `github.com/plar/go-hot-trie`
(package `hot`), Go 1.26.x, with:

- the same public interface as `github.com/plar/go-adaptive-radix-tree` (`art.Tree`),
- SIMD acceleration via Go 1.26's experimental `simd/archsimd` package (GOEXPERIMENT=simd),
  with a portable scalar fallback,
- a ported version of go-adaptive-radix-tree's test suite passing,
- benchmarks demonstrating HOT beating ART on lookups (words / HSK / UUID datasets).

Concurrency (TODS §6 ROWEX protocol) is **out of scope** — like plar/go-adaptive-radix-tree,
the tree is not safe for concurrent use.

## Background: the HOT algorithm (paper §3, §5)

A HOT tree is a tree of *compound nodes*. Each compound node represents a binary Patricia
trie with fanout ≤ k (k = 32, matching AVX2: 32 8-bit lanes in a 256-bit register). A node
storing n entries (2 ≤ n ≤ k) has n−1 implicit BiNodes; entries are child pointers (boundary
entries) or leaves (hoisted directly into the node). Every node n has height
h(n) = 1 + max(h(child)); leaves have height 0. HOT maintains provably minimal height,
determinism w.r.t. insertion order, and recursive structure.

### Linearized node layout (ALL — Adaptive Linearized node Layout)

Per node:

- **Discriminative bit positions** — set of ≤ k−1 = 31 bit positions (in encoded-key space).
  Stored compiled as an *extraction spec*:
  - *single-mask*: byte offset + 64-bit mask over an 8-byte window (used when all positions
    fall in one 8-byte window) — paper's Fig. 14(a);
  - *multi-mask*: list of (byte offset, 8-bit mask) pairs — paper's Fig. 14(b–d).
- **Sparse partial keys** — one W-bit integer per entry, W ∈ {8, 16, 32} chosen adaptively
  as the smallest width ≥ number of bits. Bit for the smallest discriminative position is
  stored most-significant (so integer order = key order; keys are strictly ascending).
  Sparse semantics: bit set ⇔ the BiNode at that position lies on the entry's path from the
  node's internal root *and* the entry took the right (1) branch there. All other bits are 0.
- **Values** — n child pointers in key order (`unsafe.Pointer` to leaf or node, discriminated
  by a common header byte).

**Search within node**: extract the *dense* partial key of the search key (read the window /
scattered bytes, PEXT-style compress — scalar emulation of PEXT), then select the largest
index i with `dense & sparse[i] == sparse[i]`. SIMD: broadcast + AND + compare-equal +
move-mask + bit-scan-reverse — one Uint8x32 op for W=8, two Uint16x16 for W=16, four
Uint32x8 for W=32. Descend; at the leaf validate the full key (Patricia search is optimistic).

### Insert (paper §3.2, Listing 1)

1. Encode key; descend recording the path; candidate leaf; if equal → update in place.
2. Mismatch bit `mbit` = first differing bit between encoded candidate and encoded new key.
3. Affected node = node containing the *mismatching BiNode* (topmost BiNode on the search
   path with position > mbit; if none, the candidate leaf entry itself, in the last node).
4. Cases:
   - **Leaf pushdown**: mismatching entity is a leaf entry in a node with h > 1 → replace the
     leaf entry with a new height-1 node holding {old leaf, new leaf}.
   - **Normal insert**: insert the discriminating BiNode into the affected node: compute
     affected entry range [lo,hi] (entries whose sparse keys match the candidate's sparse key
     on all positions < mbit — a contiguous range); add mbit to the position set if missing
     (recode sparse keys: insert 0-bit at its rank; widen W if needed); if the new key's bit
     at mbit is 1, insert new entry after hi, else set the mbit bit in affected entries and
     insert before lo.
   - **Overflow** (node already has k entries): split the node at its internal root BiNode
     (rank-0 bit) into two halves (per half: drop rank-0 bit, drop now-unused positions =
     positions where OR of the half's sparse keys has 0; recompute spec/width/height); insert
     the pending entry into the proper half; then:
     - h(split)+1 == h(parent) → **parent pull-up**: replace the entry for the old node in
       the parent with two entries (left, right half) discriminated by the old root position
       — may overflow the parent, recurse; at the root create a new root (only way height grows);
     - h(split)+1 < h(parent) → **intermediate node creation**: new 2-entry node of height
       h(split)+1 replaces the old entry in the parent.

Structural node modifications are performed on a small builder (slices of sparse keys +
children + explicit position list) and materialized into fixed-size node structs.

### Delete (paper §3.2.2, Listing 2)

- **Normal delete**: remove entry; drop positions no longer used (OR of remaining sparse
  keys — stale-but-truthful set bits are kept, they cannot cause false search results);
  a 2-entry node collapses to its remaining entry (inverse leaf pushdown).
- **Underflow** (sibling of the affected node under its parent BiNode is a single boundary
  or leaf entry, and count(n)+count(s) ≤ k):
  - equal heights → **node merge** (inverse parent pull-up),
  - sibling lower height → **simple BiNode pull-down** (n absorbs the parent BiNode and the
    sibling as one entry);
  both remove an entry from the parent and may recurse upward.

## Key encoding (variable-length, arbitrary bytes)

Patricia over bit strings requires prefix-free keys. Keys use the standard order-preserving
encoding: terminator `0x00 0x01` appended; embedded `0x00` bytes escaped as `0x00 0xFF`.
Encoded order == bytewise order of original keys, so iteration order is correct and keys
with a given prefix are contiguous.

Fast path: keys without NUL bytes are read *virtually* (no copy): byte i beyond the key is
0x00 at len, 0x01 at len+1, 0x00-padding after. Keys containing NUL are materialized (rare).
Leaves store the original key; Search compares original keys directly.

## Public API

Mirrors `art` exactly: `Key []byte`, `Value any`, `Tree` interface (Insert, Delete, Search,
ForEach, ForEachPrefix, Iterator, Minimum, Maximum, Size), `Callback`, `Node` interface
(Kind/Key/Value), `Iterator` (HasNext/Next + ErrConcurrentModification via tree version),
traverse options (TraverseLeaf/Node/All/Reverse), `New() Tree`.

Kinds differ from ART (HOT has no Node4/16/48/256): `Leaf`, `Node8`, `Node16`, `Node32`
(named by sparse-partial-key width). Ported tests that assert ART node kinds are adapted;
structure-stat tests use HOT node counts (stable across insertion orders by the determinism
property, which is itself tested).

## Package layout

- `api.go` — public API (mirrors art's api.go)
- `key.go` — encoding, virtual byte reads, mismatch-bit computation
- `node.go` — header, leaf, generic `node[W]` (fixed arrays: `[32]W` partial keys,
  `[32]unsafe.Pointer` children, extraction spec), kind dispatch
- `extract.go` — dense-key extraction, scalar PEXT/PDEP-style helpers
- `search_simd.go` (`//go:build goexperiment.simd && amd64`) — archsimd match kernels
- `search_scalar.go` (`//go:build !(goexperiment.simd && amd64)`) — fallback
- `tree.go`, `insert.go`, `delete.go`, `traversal.go`, `iterator.go`
- `builder.go` — node (re)construction for structural modifications
- tests: ported ART suite + HOT invariant checks (k-constraint, height minimality vs
  reference partitioning, determinism, sparse-key semantics) + fuzz-ish randomized tests
- `benchmark/` — separate Go module comparing against `plar/go-adaptive-radix-tree/v2`
  (keeps the main module dependency-free except testify)

## Testing plan

1. Port all behavioral tests from tree_test.go, tree_traversal_test.go (adapted kinds).
2. HOT-specific: determinism (shuffle-insert ⇒ identical structure), invariant walker
   (entry counts 2..32, heights consistent, sparse keys strictly ascending, sparse bits
   truthful vs actual leaf keys), delete-then-verify height invariants.
3. Randomized: N random variable-length keys (with NULs) insert/search/delete vs a map +
   sorted-order iteration vs sort.Slice reference.
4. Benchmarks in `benchmark/`: Insert + Search on words/hsk/uuid for ART vs HOT; goal:
   HOT Search faster than ART on all three datasets.

## Implementation deviations

- **PEXT emulation by mask runs.** With no BMI2 intrinsic in Go, dense
  partial keys are extracted via precomputed contiguous runs of the window
  mask (typically ≤ 6 per node, a few shift/mask/or steps each). Multi-mask
  nodes with ≤ 8 scattered mask bytes gather them into one synthesized
  window first; only nodes with > 8 scattered mask bytes use the per-byte
  scalar PEXT loop.
- **Lazy position reclamation on delete.** In-place normal deletion leaves a
  vanished BiNode's position in the spec (all sparse bits at the dead rank
  are zero, so matching is unaffected); `load()` reclaims dead positions
  whenever a builder-based structural operation touches the node, so splits,
  merges and width migrations always see exact minimal position sets.
- **Raw-key fast path.** Search and Delete descend on the raw key bytes and
  only retry with the escaped encoding when a NUL-containing key misses;
  leaf validation makes wrong candidates harmless. Leaves cache a
  "contains NUL" flag in the spare header byte so insertion escapes
  candidate keys without rescanning them.
- **AVX2-only SIMD.** `Mask16x16/Mask32x8.ToBits` encode to AVX-512 mask
  instructions, so the 16/32-bit kernels compare partial keys bytewise
  (a lane matches iff all its bytes match) and fold the byte bitmap.
- **Hardware PEXT in scalar builds only.** A BMI2 `PEXT` assembly routine
  (CPUID-gated) replaces the mask-run loop when the SIMD kernels are not
  compiled in (~10% search win). In GOEXPERIMENT=simd builds the same call
  regressed search 6x: the compiler must treat ABI0 assembly calls as
  clobbering all vector registers, and spilling the live SIMD state of the
  descent loop cost ~100ns per call (measured via an A/B benchmark with a
  pure-Go no-inline callee, which showed only normal call overhead). SIMD
  builds therefore keep the run-based emulation.

## SIMD notes

`simd/archsimd` exists only under GOEXPERIMENT=simd (Go 1.26); files are build-tagged so
the library works on any platform/toolchain, and CI/benchmarks run with GOEXPERIMENT=simd.
AVX2 availability is checked at runtime via `archsimd.X86.AVX2()` once at init; if absent,
scalar kernels are used even in SIMD builds.
