//go:build amd64

#include "textflag.h"

// func cpuid7ebx() uint32
TEXT ·cpuid7ebx(SB), NOSPLIT, $0-4
	XORL	AX, AX
	XORL	CX, CX
	CPUID
	CMPL	AX, $7
	JLT	zero
	MOVL	$7, AX
	XORL	CX, CX
	CPUID
	MOVL	BX, ret+0(FP)
	RET
zero:
	MOVL	$0, ret+0(FP)
	RET

// func pext64asm(x, mask uint64) uint64
TEXT ·pext64asm(SB), NOSPLIT, $0-24
	MOVQ	x+0(FP), AX
	MOVQ	mask+8(FP), BX
	PEXTQ	BX, AX, AX
	MOVQ	AX, ret+16(FP)
	RET
