# Changelog

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
