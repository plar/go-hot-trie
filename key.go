package hot

import (
	"bytes"
	"encoding/binary"
	"math/bits"
)

// Keys are indexed by the bits of an order-preserving, prefix-free encoding:
// every NUL byte is escaped as 0x00 0xFF and a two byte terminator 0x00 0x01
// is (virtually) appended. The terminator and the trailing zero padding are
// never materialized: escapeKey returns the escaped body and keyByteAt serves
// the terminator/padding on demand. For keys without NUL bytes escapeKey is a
// zero-copy identity, which keeps the search path allocation free.
//
// Bit positions are numbered over the encoded key with bit 0 being the most
// significant bit of byte 0.

var (
	escNul     = []byte{0x00}
	escNulRepl = []byte{0x00, 0xFF}
)

// escapeKey returns the encoded body of the key (without the terminator).
func escapeKey(key Key) []byte {
	if bytes.IndexByte(key, 0) < 0 {
		return key
	}
	return bytes.ReplaceAll(key, escNul, escNulRepl)
}

// keyByteAt reads byte i of the fully encoded key given its escaped body.
func keyByteAt(ek []byte, i int) byte {
	if i < len(ek) {
		return ek[i]
	}
	if i == len(ek)+1 {
		return 0x01
	}
	return 0
}

// keyBitAt returns the bit at position p of the encoded key.
func keyBitAt(ek []byte, p uint32) uint32 {
	b := keyByteAt(ek, int(p>>3))
	return uint32(b>>(7-p&7)) & 1
}

// mismatchBit returns the position of the first differing bit between two
// encoded keys, or -1 if they are equal. The shared prefix is compared a
// word at a time; the tail and the virtual terminator byte by byte.
func mismatchBit(a, b []byte) int {
	i := 0
	for n := min(len(a), len(b)); i+8 <= n; i += 8 {
		x := binary.BigEndian.Uint64(a[i:])
		y := binary.BigEndian.Uint64(b[i:])
		if x != y {
			return i*8 + bits.LeadingZeros64(x^y)
		}
	}
	for n := max(len(a), len(b)) + 2; i < n; i++ {
		x := keyByteAt(a, i)
		y := keyByteAt(b, i)
		if x != y {
			return i*8 + bits.LeadingZeros8(x^y)
		}
	}
	return -1
}
