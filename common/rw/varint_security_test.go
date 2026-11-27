package rw_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/metacubex/sing/common/rw"
)

func TestReadVStringBoundedAndWriteUVariant(t *testing.T) {
	// 1. Negative int64 cast (bit 63 set) must return error without panicking.
	maxUvarint := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}
	if _, err := rw.ReadVString(bytes.NewReader(maxUvarint)); err == nil {
		t.Fatalf("expected error for max uint64 vstring length")
	}

	// 2. Oversized length exceeding MaxVStringLen must fail before allocation.
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], rw.MaxVStringLen+1)
	if _, err := rw.ReadVString(bytes.NewReader(buf[:n])); err == nil {
		t.Fatalf("expected error for vstring exceeding MaxVStringLen")
	}

	// 3. WriteUVariant and UvarintLen must handle >= 1<<56 without panicking.
	for _, v := range []uint64{0, 1 << 7, 1 << 55, 1 << 56, ^uint64(0)} {
		var out bytes.Buffer
		if err := rw.WriteUVariant(&out, v); err != nil {
			t.Fatalf("WriteUVariant(%d): %v", v, err)
		}
		if out.Len() != rw.UvarintLen(v) {
			t.Fatalf("UvarintLen(%d)=%d, actual=%d", v, rw.UvarintLen(v), out.Len())
		}
		got, err := rw.ReadUVariant(&out)
		if err != nil || got != v {
			t.Fatalf("ReadUVariant(%d)=%d, err=%v", v, got, err)
		}
	}
}
