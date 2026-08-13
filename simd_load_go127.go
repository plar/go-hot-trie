//go:build goexperiment.simd && amd64 && go1.27

package hot

import "simd/archsimd"

// loadu8x32 abstracts the archsimd load whose signature changed between Go
// 1.26 (*[32]uint8) and Go 1.27 ([]uint8).
func loadu8x32(p *[32]uint8) archsimd.Uint8x32 {
	return archsimd.LoadUint8x32(p[:])
}
