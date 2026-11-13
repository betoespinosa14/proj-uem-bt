package exec

import (
	"testing"
	"unsafe"
)

func TestWTSString(t *testing.T) {
	p, n := wtsString("Hi ✓")
	if n != 8 { // 4 UTF-16 units, 2 bytes each, terminator not counted
		t.Fatalf("length = %d bytes, want 8", n)
	}
	units := unsafe.Slice(p, 5)
	if units[0] != 'H' || units[3] != '✓' || units[4] != 0 {
		t.Fatalf("units = %v", units)
	}
}
