package gpdf

import (
	"reflect"
	"testing"
)

// The engine allocates these per glyph and per drawing run -- a two-page
// quotation runs to several thousand -- so their layout is the one place in this
// codebase where field order is a real cost rather than a habit.
//
// Go does not reorder struct fields. A field placed after a wider one leaves
// padding behind, and a pointer placed after a word leaves the garbage collector
// scanning bytes it could have skipped. Both are silent: nothing fails, the
// memory is simply wasted, and the next person to add a field in the "logical"
// position undoes the work without any signal.
//
// These tests are that signal. They assert pointer-containing fields come before
// pointer-free ones, and that the types stay within a byte budget, so a field
// added in the wrong place fails here instead of costing memory silently.
//
// What this does not claim: the reorder did not shrink these types. box is 104
// bytes before and after. The win is the collector's scan window, not the
// allocation.

// pointerFree reports whether t contains no pointer, so the collector has no
// reason to look at it.
func pointerFree(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return true
	case reflect.Array:
		return pointerFree(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if !pointerFree(t.Field(i).Type) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// TestHotTypesArePointerFirst pins the collector-facing half of the layout.
func TestHotTypesArePointerFirst(t *testing.T) {
	for _, v := range []any{
		box{}, run{}, picture{}, clippedFill{}, gradient{}, glyphRun{},
		totalRow{}, Face{}, Engine{}, lazy{},
	} {
		typ := reflect.TypeOf(v)
		t.Run(typ.Name(), func(t *testing.T) {
			seenPointerFree := false
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				if pointerFree(f.Type) {
					seenPointerFree = true
					continue
				}
				if seenPointerFree {
					t.Errorf("field %s (%s) holds a pointer but follows a pointer-free field, "+
						"so the collector scans past it; move it above %s",
						f.Name, f.Type, typ.Field(i-1).Name)
				}
			}
		})
	}
}

// TestHotTypeSizes pins the total-size half, as a regression budget rather than
// as a claim about what the reorder achieved. These are the current sizes; the
// field reorder did not shrink them, because these types were already at their
// size floor once the collector-facing prefix is fixed. box is 104 bytes before
// and after.
//
// What the reorder did buy here is the pointer prefix, which
// TestHotTypesArePointerFirst covers. The two are different axes and conflating
// them would make this test look like it is guarding something it is not.
//
// Engine's budget is 384, not 144: since issue #335 it carries the resolved
// theme.Palette (nineteen colour strings) by value instead of four base
// strings. That is 240 bytes more on a value copied once per render, against
// a ~55KB document. The palette is carried by value deliberately: concurrent
// renders each work on their own Engine copy via WithTheme (see engine.go),
// so no lock or atomic is needed. A pointer would also work — it would make
// replacement atomic by swapping one word — but it would add an allocation,
// an indirection on every paint, and a nil to guard, for a value that is
// already copied cheaply once per render. The budget records the size; it
// does not forbid growing it for a reason.
func TestHotTypeSizes(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  uintptr
	}{
		{box{}, 104}, {run{}, 72}, {picture{}, 56}, {clippedFill{}, 72},
		{gradient{}, 88}, {glyphRun{}, 40}, {totalRow{}, 56},
		{Face{}, 104}, {Engine{}, 384}, {lazy{}, 40},
	} {
		typ := reflect.TypeOf(tc.value)
		if got := typ.Size(); got > tc.want {
			t.Errorf("%s is %d bytes, budget is %d; a field was added in a position that pads", typ.Name(), got, tc.want)
		}
	}
}
