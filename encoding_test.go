package bindiff

import (
	"bytes"
	"math"
	"testing"
)

func TestSignMagnitudeLittleEndian(t *testing.T) {
	tests := []struct {
		value int64
		bytes []byte
	}{
		{0, []byte{0, 0, 0, 0, 0, 0, 0, 0}},
		{1, []byte{1, 0, 0, 0, 0, 0, 0, 0}},
		{-1, []byte{1, 0, 0, 0, 0, 0, 0, 0x80}},
		{0x0102030405060708, []byte{8, 7, 6, 5, 4, 3, 2, 1}},
		{-0x0102030405060708, []byte{8, 7, 6, 5, 4, 3, 2, 0x81}},
		{math.MaxInt64, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}},
		{-math.MaxInt64, []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
	}

	var order signMagnitudeLittleEndian
	for _, tt := range tests {
		b := make([]byte, 8)
		order.PutUint64(b, uint64(tt.value))
		if !bytes.Equal(b, tt.bytes) {
			t.Errorf("PutUint64(%d) = % x, want % x", tt.value, b, tt.bytes)
		}

		if got := int64(order.Uint64(tt.bytes)); got != tt.value {
			t.Errorf("Uint64(% x) = %d, want %d", tt.bytes, got, tt.value)
		}
	}
}

func TestSignMagnitudeNegativeZero(t *testing.T) {
	var order signMagnitudeLittleEndian
	if got := order.Uint64([]byte{0, 0, 0, 0, 0, 0, 0, 0x80}); got != 0 {
		t.Fatalf("Uint64(-0) = %d, want 0", got)
	}
}
