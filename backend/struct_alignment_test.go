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
	// The sub-item fields (a slice and a flag, #255) grew the struct from
	// 56 to 72 bytes legitimately: the struct now describes a modular
	// line, not only a flat one. 72 is its size floor with the slice
	// leading -- the aligned field order at the type says so -- so the
	// budget pins 72 and a future field added in a padding position
	// fails here rather than slipping past the analyzer.
	got := unsafe.Sizeof(QuotationItemInput{})
	if got != 72 {
		t.Errorf("QuotationItemInput is %d bytes, want 72; the field order is no longer at its size floor "+
			"(pointer-bearing fields must lead; a field added in a padding position costs every line item).", got)
	}
	if class := classOf(got); class != 80 {
		t.Errorf("QuotationItemInput lands in size class %d, want 80.", class)
	}
}
