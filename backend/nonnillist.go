package backend

// NonNilList keeps a decoded list from crossing the Wails boundary as a nil
// slice. Go marshals a nil slice to JSON null, and the desktop reads .length
// and iterates these directly: null there throws during render, and with no
// error boundary that unmounted the entire window (the blank screen after
// Prepare). Returning [] instead makes the wire shape match the TypeScript
// types the frontend already declares.
//
// This is for lists that cross into the UI. It is deliberately NOT applied to
// the signing path: the canonical payload is hashed byte-for-byte, so changing
// null to [] there would stop every already-printed verification code from
// verifying.
func NonNilList[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
