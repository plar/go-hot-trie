//go:build !(goexperiment.simd && amd64)

package hot

// Fallback used when the simd package is unavailable (non-amd64 platforms or
// builds without GOEXPERIMENT=simd).

func simdEnabled() bool { return false }

// simdBuild marks builds with the SIMD match kernels compiled in.
const simdBuild = false

func match8(pks *[maxFanout]uint8, dense uint8, num uint8) int {
	return match8scalar(pks, dense, num)
}

func match16(pks *[maxFanout]uint16, dense uint16, num uint8) int {
	return match16scalar(pks, dense, num)
}

func match32(pks *[maxFanout]uint32, dense uint32, num uint8) int {
	return match32scalar(pks, dense, num)
}
