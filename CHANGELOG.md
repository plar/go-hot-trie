# Changelog

## v0.1.2 (2026-08-13)

- Test suite hardening from a three-way expert review: differential fuzz
  oracle (counter values, final re-search of every key, prefix/reverse/min/
  max checks, live-iterator op, long-key derivation, structural seeds), a
  FuzzDeterminism target, a unit layer for the extraction and builder
  internals, directed underflow/churn/long-key/deep-tree tests, SIMD-toggle
  and BMI2 fallback coverage, zero-allocation search guard, API contract
  table, and per-delete height-consistency scans.
- Hardened fixHeightsFrom: the ancestor height walk no longer early-breaks
  across hoist-replaced levels; removed two unreachable branches.
- Documented and race-tested the read-only concurrency contract.
- Benchmarks now measure (the ported Iterator/ForEach benchmarks ignored
  b.N); structure assertions moved to tests; ReportAllocs everywhere.
- Nightly fuzz workflow (3 targets x scalar/SIMD) with a persistent corpus
  cache; make fuzz covers the SIMD build.
- Delete-path height maintenance: resolveUnderflow now starts the ancestor
  height walk at the first unmodified ancestor, restoring the minimal
  early-break walk with a documented contract.
- Fuzz iterator oracle rewritten to O(1) per op (strictly increasing
  ref-member keys instead of a sorted snapshot), protecting fuzz throughput;
  invariant cadence scales with tree size.
- Test oracles consolidated (shared prefix-scan, order, and shape helpers);
  in-package delete benchmarks added.

## v0.1.1 (2026-08-13)

- Scoped the README claims to what the implementation guarantees: insertion
  produces provably minimal height; deletion keeps the height close to
  minimal through node merges.
- Renamed benchmark/REPORT.md to benchmark/README.md so GitHub renders it in
  the directory view.

## v0.1.0 (2026-08-13)

Initial release. HOT (Binna et al., SIGMOD 2018 / ACM TODS 2022) with the
go-adaptive-radix-tree interface, SIMD lookups behind `GOEXPERIMENT=simd`
on Go 1.26 and 1.27 with scalar and BMI2 PEXT fallbacks, fuzz and invariant
test suites, height-minimality and determinism verification, and
head-to-head ART benchmarks.
