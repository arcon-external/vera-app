package backend

import (
	"testing"
	"unsafe"
)

// Most structs in this package were reordered for the garbage collector, not for
// size: putting pointer-bearing fields first shrinks the scan window, and that is
// where the win is on types allocated thousands of times (see
// backend/render/gpdf/alignment_test.go for the renderer's hot types).
//
// Two types are different, and they are the only ones in the tree where a
// reorder actually returns fewer bytes rather than fewer scanned bytes. Go
// allocates in size classes, and a reorder only pays if it crosses one:
//
//	QuotationInput      152 bytes -> class 160   ...  144 bytes -> class 144
//	QuotationItemInput   56 bytes -> class  64   ...   48 bytes -> class  48
//
// Everything else the analyzer flagged lands in the same class either way --
// ArchivedQuotation is 592 or 584, both out of a 640-byte bucket -- so it costs a
// large diff and returns nothing. These tests pin the two that pay, so a field
// added in a padding position fails instead of quietly costing 16 bytes per
// item on every quotation.

// classOf is the Go allocator size class n falls into for the sizes here.
func classOf(n uintptr) uintptr {
	for _, c := range []uintptr{8, 16, 24, 32, 48, 64, 80, 96, 112, 128, 144, 160, 176, 192, 208, 224, 240, 256, 288, 320, 352, 384, 416, 448, 480, 512, 576, 640, 704, 768, 896, 1024} {
		if n <= c {
			return c
		}
	}
	return n
}

func TestQuotationInputStaysInTheSmallerSizeClass(t *testing.T) {
	got := unsafe.Sizeof(QuotationInput{})
	if class := classOf(got); class != 144 {
		t.Errorf("QuotationInput is %d bytes, landing in size class %d; the reordering put it in class 144. "+
			"A field was added in a position that costs 16 bytes per quotation.", got, class)
	}
}

func TestQuotationItemInputStaysInTheSmallerSizeClass(t *testing.T) {
	// The budget has moved twice, and each time for content rather than for
	// padding:
	//
	//   56 -> 72  sub-items: a slice and a flag (#255). The struct now
	//                describes a modular line, not only a flat one.
	//   72 -> 80  a per-row discount: a uint16 and a bool (#274).
	//
	// 80 is the floor for the present field set, not a slip. The content is
	// 16+16+24+8+4+2+2+3 = 75 bytes, which rounds to 80 at any field order,
	// and even packing all three independent booleans into a single flag byte
	// still lands on 80 (73 content bytes). So this number cannot go back to
	// 72 without dropping a feature, and the test's real job is unchanged:
	// catch the next field being added in a *padding* position, where it
	// would cost 8 or 16 bytes for nothing.
	got := unsafe.Sizeof(QuotationItemInput{})
	if got != 80 {
		t.Errorf("QuotationItemInput is %d bytes, want 80; the field order is no longer at its size floor "+
			"(pointer-bearing fields must lead; a field added in a padding position costs every line item).", got)
	}
	if class := classOf(got); class != 80 {
		t.Errorf("QuotationItemInput lands in size class %d, want 80.", class)
	}
}
