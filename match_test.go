package hot

import (
	"math/rand"
	"testing"
)

// TestMatchKernelsAgree verifies the (possibly SIMD) match kernels against
// the scalar reference on random inputs. Under GOEXPERIMENT=simd on amd64
// this exercises the AVX2 paths; otherwise it is a self-check of the
// fallback wiring.
func TestMatchKernelsAgree(t *testing.T) {
	t.Logf("simdEnabled=%v", simdEnabled())
	rng := rand.New(rand.NewSource(1))
	for range 20000 {
		num := uint8(rng.Intn(31) + 2)

		var p8 [maxFanout]uint8
		for i := 1; i < int(num); i++ {
			p8[i] = uint8(rng.Uint32())
		}
		d8 := uint8(rng.Uint32())
		if got, want := match8(&p8, d8, num), match8scalar(&p8, d8, num); got != want {
			t.Fatalf("match8: got %d want %d (dense=%02x num=%d pks=%v)", got, want, d8, num, p8[:num])
		}

		var p16 [maxFanout]uint16
		for i := 1; i < int(num); i++ {
			p16[i] = uint16(rng.Uint32())
		}
		d16 := uint16(rng.Uint32())
		if got, want := match16(&p16, d16, num), match16scalar(&p16, d16, num); got != want {
			t.Fatalf("match16: got %d want %d (dense=%04x num=%d pks=%v)", got, want, d16, num, p16[:num])
		}

		var p32 [maxFanout]uint32
		for i := 1; i < int(num); i++ {
			p32[i] = rng.Uint32()
		}
		d32 := rng.Uint32()
		if got, want := match32(&p32, d32, num), match32scalar(&p32, d32, num); got != want {
			t.Fatalf("match32: got %d want %d (dense=%08x num=%d pks=%v)", got, want, d32, num, p32[:num])
		}
	}
}
