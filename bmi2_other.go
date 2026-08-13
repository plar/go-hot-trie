//go:build !amd64

package hot

const useBMI2 = false

func pext64asm(x, mask uint64) uint64 { panic("pext64asm without BMI2") }
