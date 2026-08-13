//go:build goexperiment.simd && amd64

package hot

import (
	"math/bits"
	"simd/archsimd"
	"unsafe"
)

// SIMD sparse partial key matching (paper Listing 4): broadcast the dense
// partial key, AND it with all sparse partial keys, compare for equality with
// the sparse keys, turn the resulting mask into a bitmap and pick the highest
// set bit within the used-entries range (bit scan reverse).
//
// All kernels stick to AVX2-encodable operations: byte-granular compares and
// VPMOVMSKB (Mask8x32.ToBits). Wider-lane ToBits variants encode to AVX-512
// mask instructions, so 16- and 32-bit partial keys are compared bytewise
// instead: a lane matches iff all of its bytes match, which the bitmap
// AND-fold below checks.
var useSIMD = archsimd.X86.AVX2()

// simdBuild marks builds with the SIMD match kernels compiled in.
const simdBuild = true

// simdEnabled reports whether the SIMD kernels are compiled in and usable on
// this CPU.
func simdEnabled() bool { return useSIMD }

func match8(pks *[maxFanout]uint8, dense uint8, num uint8) int {
	if !useSIMD {
		return match8scalar(pks, dense, num)
	}
	v := loadu8x32(pks)
	b := archsimd.BroadcastUint8x32(dense)
	m := v.And(b).Equal(v).ToBits()
	m &= uint32(1)<<num - 1
	return bits.Len32(m) - 1
}

func match16(pks *[maxFanout]uint16, dense uint16, num uint8) int {
	if !useSIMD {
		return match16scalar(pks, dense, num)
	}
	b := archsimd.BroadcastUint16x16(dense).AsUint8x32()
	lo := loadu8x32((*[32]uint8)(unsafe.Pointer(&pks[0])))
	m := uint64(lo.And(b).Equal(lo).ToBits())
	if num > 16 {
		hi := loadu8x32((*[32]uint8)(unsafe.Pointer(&pks[16])))
		m |= uint64(hi.And(b).Equal(hi).ToBits()) << 32
	}
	m &= m >> 1 // entry matches iff both of its bytes match
	m &= 0x5555555555555555
	m &= uint64(1)<<(2*uint(num)) - 1 // num==32 wraps to an all-ones mask
	return (bits.Len64(m) - 1) >> 1
}

func match32(pks *[maxFanout]uint32, dense uint32, num uint8) int {
	if !useSIMD {
		return match32scalar(pks, dense, num)
	}
	b := archsimd.BroadcastUint32x8(dense).AsUint8x32()
	n := int(num)
	for j := (n - 1) / 8; j >= 0; j-- {
		v := loadu8x32((*[32]uint8)(unsafe.Pointer(&pks[j*8])))
		m := v.And(b).Equal(v).ToBits()
		m &= m >> 1 // entry matches iff all four of its bytes match
		m &= m >> 2
		m &= 0x11111111
		used := min(n-j*8, 8)
		m &= uint32(1)<<(4*uint(used)) - 1 // used==8 wraps to all ones
		if m != 0 {
			return j*8 + (bits.Len32(m)-1)>>2
		}
	}
	return 0
}
