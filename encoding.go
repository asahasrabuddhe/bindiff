package bindiff

// signMagnitudeLittleEndian is the integer encoding bsdiff uses: 64-bit
// little-endian with the top bit as the sign. It implements
// binary.ByteOrder so it works with binary.Read and binary.Write, which only
// call the 64-bit methods for the int64 fields in a patch.
type signMagnitudeLittleEndian struct{}

func (s signMagnitudeLittleEndian) Uint16([]byte) uint16 {
	panic("bindiff: only 64-bit values are sign-magnitude encoded")
}

func (s signMagnitudeLittleEndian) Uint32([]byte) uint32 {
	panic("bindiff: only 64-bit values are sign-magnitude encoded")
}

func (s signMagnitudeLittleEndian) Uint64(b []byte) uint64 {
	val := int64(b[0]) |
		int64(b[1])<<8 |
		int64(b[2])<<16 |
		int64(b[3])<<24 |
		int64(b[4])<<32 |
		int64(b[5])<<40 |
		int64(b[6])<<48 |
		int64(b[7]&0x7f)<<56

	if b[7]&0x80 != 0 {
		val = -val
	}

	return uint64(val)
}

func (s signMagnitudeLittleEndian) PutUint16([]byte, uint16) {
	panic("bindiff: only 64-bit values are sign-magnitude encoded")
}

func (s signMagnitudeLittleEndian) PutUint32([]byte, uint32) {
	panic("bindiff: only 64-bit values are sign-magnitude encoded")
}

func (s signMagnitudeLittleEndian) PutUint64(b []byte, v uint64) {
	val := int64(v)
	neg := val < 0
	if neg {
		val = -val
	}

	b[0] = byte(val)
	b[1] = byte(val >> 8)
	b[2] = byte(val >> 16)
	b[3] = byte(val >> 24)
	b[4] = byte(val >> 32)
	b[5] = byte(val >> 40)
	b[6] = byte(val >> 48)
	b[7] = byte(val >> 56)

	if neg {
		b[7] |= 0x80
	}
}

func (s signMagnitudeLittleEndian) String() string {
	return "signMagnitudeLittleEndian"
}
