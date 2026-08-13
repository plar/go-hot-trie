//go:build amd64

package hot

// hasBMI2 reports hardware support for the PEXT instruction.
var hasBMI2 = cpuid7ebx()&(1<<8) != 0

// useBMI2 gates the hardware PEXT-based dense partial key extraction
// (paper §5.1.2 uses _pext_u64 directly; Go has no BMI2 intrinsic, so the
// single instruction is wrapped in an assembly routine). When false, the
// precomputed mask-run emulation in extract is used instead.
//
// It is disabled in SIMD builds: under GOEXPERIMENT=simd the compiler has
// to treat a call to an ABI0 assembly function as clobbering all vector
// registers, and with the SIMD match kernels keeping vector state live in
// the descent loop the resulting spill/transition cost (~100ns per call)
// far exceeds what PEXT saves. In scalar builds the same call replaces the
// mask-run loop profitably (~10% faster search).
var useBMI2 = hasBMI2 && !simdBuild

// cpuid7ebx returns CPUID.(EAX=7,ECX=0):EBX, or 0 if leaf 7 is unsupported.
// Implemented in bmi2_amd64.s.
func cpuid7ebx() uint32

// pext64asm executes PEXT: the bits of x selected by mask, packed towards
// the least significant bit. Must only be called when useBMI2 is true.
// Implemented in bmi2_amd64.s.
func pext64asm(x, mask uint64) uint64
