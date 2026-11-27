package buf_test

import (
	"testing"

	"github.com/metacubex/sing/common/buf"
)

func TestAllocatorPutInvalidSizesNoPanic(t *testing.T) {
	for _, size := range []int{0, 1, 2, 4, 8, 16, 32, 33, 65, 131072} {
		var b []byte
		if size > 0 {
			b = make([]byte, size)
		}
		if err := buf.Put(b); err == nil {
			t.Fatalf("expected error for invalid buffer cap %d", size)
		}
	}
	for _, size := range []int{64, 128, 65536} {
		b := buf.Get(size)
		if err := buf.Put(b); err != nil {
			t.Fatalf("unexpected error for valid buffer cap %d: %v", size, err)
		}
	}
}
