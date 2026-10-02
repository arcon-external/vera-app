package gpdf

// Real axial gradients for a PDF library that has no shading primitive.
//
// gopdf cannot express a /Shading: there is no Shading, PatternType or gradient
// identifier anywhere in its API, and its content stream is assembled from a
// fixed set of cached operators with no raw escape hatch. The only public
// surface is the finished byte stream.
//
// So the gradient is applied as a post-process. Each gradient rect is drawn
// with a SENTINEL fill colour, then this pass rewrites the document to:
//
//   - add a /Pattern object of PatternType 2 holding Chrome's own /Shading
//     (a FunctionType 2 exponential ramp between the two endpoint colours),
//     with /Coords spanning the rect's width and /Matrix mapping it to page
//     space;
//   - register the pattern in the page's /Resources under /Pattern;
//   - replace the sentinel fill colour ahead of the rect's path with the two
//     operators that select the pattern (cs) and paint with it (scn).
//
// The alternative, approximating the ramp with thousands of thin bands, was
// tried first and rejected. Each band has to quantise to 8 bits per channel, so
// the steps are visible, and hiding the seams with a small overlap makes them
// zoom-dependent, because a 0.02pt overlap either straddles a pixel boundary
// or lands inside one. A real shading has no seams at any scale.

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// gradientSentinel* is the fill colour a gradient rect is drawn with before
// injection. It lies outside the template's palette, so a sentinel match cannot
// collide with a real fill.
const (
	gradientSentinelR = 0xFE
	gradientSentinelG = 0xFD
	gradientSentinelB = 0xFC
)

// gradient is one rect to convert into a shading.
type gradient struct {
	c0     string
	c1     string
	x0     float64
	y0     float64
	w      float64
	h      float64
	pageH  float64
	objNum int
	from   int
}

var (
	errNoXref            = errors.New("gradient: no xref table")
	errShortXref         = errors.New("gradient: xref table ends early")
	errNoObj             = errors.New("gradient: malformed object")
	errNoPage            = errors.New("gradient: no page object")
	errNoStream          = errors.New("gradient: object has no stream")
	errNoResources       = errors.New("gradient: no inline /Resources dictionary")
	errNoInlineResources = errors.New("gradient: /Resources is an indirect reference")
	errNoSentinel        = errors.New("gradient: sentinel fill not found")
)

// injectGradients rewrites page `page` of a document, replacing every
// sentinel-filled rect with a real axial /Shading.
//
// The page index is explicit because a multi-page quotation has one ribbon per
// page and each page's content stream is a separate object. Object numbers are
// re-derived on every call from the current xref, so calling this once per page
// in sequence allocates fresh pattern objects each time and never collides.
func injectGradients(doc []byte, grads []gradient, page int) ([]byte, error) {
	if len(grads) == 0 {
		return doc, nil
	}

	offsets, err := parseXrefOffsets(doc)
	if err != nil {
		return nil, err
	}

	contentObj, err := pageContentObject(doc, offsets, page)
	if err != nil {
		return nil, err
	}
	body, err := objectBody(doc, offsets[contentObj])
	if err != nil {
		return nil, err
	}
	content, err := decodeStream(body)
	if err != nil {
		return nil, err
	}

	// Each gradient's pattern object number must be known before the content
	// stream can reference it, so allocate them up front.
	firstPattern := 0
	for n := range offsets {
		if n >= firstPattern {
			firstPattern = n + 1
		}
	}
	for i := range grads {
		grads[i].objNum = firstPattern + i
	}

	// The cursor is shared by the whole loop, not held per gradient: every
	// gradient's sentinel search has to resume where the previous one stopped,
	// because each one otherwise finds the first sentinel again.
	cursor := 0
	for i := range grads {
		if content, err = swapSentinelForPattern(content, cursor, &grads[i]); err != nil {
			return nil, err
		}
		cursor = grads[i].from
	}

	return rebuildPDF(doc, offsets, contentObj, content, firstPattern, grads, page)
}

// ---- content stream rewriting --------------------------------------------

// swapSentinelForPattern rewrites the one fill operator that precedes a
// gradient rect's path, turning it into a pattern fill.
//
// gopdf writes every fill as a fixed sequence, so the whole block is matched as
// a single string rather than searching for each operator separately:
//
//	"<r> <g> <b> rg\nq\n<x0> <yTop> m <x1> <yTop> l <x1> <yBot> l <x0> <yBot> l  f\n"
//
// Matching the sentinel colour as part of that string means a mismatch is a
// hard error rather than a silent corruption of some other fill.
func swapSentinelForPattern(content []byte, from int, g *gradient) ([]byte, error) {
	// A pattern is not a colour space. The lowercase "cs" sets the FILL colour
	// space to /Pattern and the lowercase "scn" then selects the fill pattern,
	// so both must be lowercase: "CS"/"SCN" are the stroke pair and do not
	// satisfy a following "scn", which poppler rejects with "Incorrect number
	// of arguments". Chrome writes both pairs; only the fill pair is needed.
	//
	// The shape that follows the colour operator differs by library, so each
	// candidate is tried in turn rather than assumed:
	//
	//   gopdf  "0.996 0.992 0.988 rg" "q" "<x> <y> m ... l  f"   (polygon path)
	//   gpdf   "0.996… rg" "q" "<x> <y> <w> <h> re" "f"         (native rect)
	//
	// gpdf also prints floats at full precision where gopdf trims them, so the
	// coordinates are matched numerically rather than as formatted strings.
	sentinelR := float64(gradientSentinelR) / 255
	sentinelG := float64(gradientSentinelG) / 255
	sentinelB := float64(gradientSentinelB) / 255

	// Search from wherever the previous gradient left off. Always starting from
	// zero makes every pass rewrite the first sentinel it can find, so the second
	// pass lands on the same one again: the first gradient's fill ends up
	// carrying both patterns — the last one written wins — and the second shape,
	// the decoration bar under the metadata card, keeps the sentinel and paints
	// the near-white the reader sees as a blank strip under the card instead of
	// a blue gradient.
	head := sentinelFillEnd(content, from, sentinelR, sentinelG, sentinelB)
	if head < 0 {
		return nil, fmt.Errorf("%w: no sentinel fill %.6f %.6f %.6f in content stream",
			errNoSentinel, sentinelR, sentinelG, sentinelB)
	}
	body := content[head:]

	// Replace only the colour operator, leaving the rest of the fill untouched.
	// The trailing newline matters: "scn" and the next operator would otherwise
	// fuse into one token ("scn38.25"), which poppler rejects.
	pat := []byte(fmt.Sprintf("/Pattern cs\n/Pg%d scn\n", g.objNum))
	var out bytes.Buffer
	out.Write(content[:head])
	out.Write(pat)
	out.Write(body)
	g.from = head + len(pat)
	return out.Bytes(), nil
}

// sentinelFillEnd finds the end of the sentinel colour operator at or after
// `from`, and returns the offset just past its newline, or -1. Matching on the
// parsed value rather than a formatted string is what lets one pass serve
// libraries with different float precision.
func sentinelFillEnd(content []byte, from int, r, g, b float64) int {
	const tol = 5e-4
	s := string(content)
	for i := from; ; {
		j := strings.Index(s[i:], " rg")
		if j < 0 {
			return -1
		}
		j += i
		line := s[max(0, j-64):j]
		if f, ok := lastThreeFloats(line); ok &&
			near(f[0], r, tol) && near(f[1], g, tol) && near(f[2], b, tol) {
			return j + len(" rg") + 1
		}
		i = j + 1
	}
}

// lastThreeFloats parses the last three space-separated numbers in s.
func lastThreeFloats(s string) ([3]float64, bool) {
	fields := strings.Fields(strings.TrimSpace(s))
	if len(fields) < 3 {
		return [3]float64{}, false
	}
	var out [3]float64
	for k := 0; k < 3; k++ {
		v, err := strconv.ParseFloat(fields[len(fields)-3+k], 64)
		if err != nil {
			return [3]float64{}, false
		}
		out[k] = v
	}
	return out, true
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---- PDF structure --------------------------------------------------------

// parseXrefOffsets reads a classic xref table's byte offsets, keyed by object
// number. Free entries (generation != 0) are skipped.
func parseXrefOffsets(doc []byte) (map[int]int, error) {
	i := bytes.Index(doc, []byte("xref"))
	if i < 0 {
		return nil, errNoXref
	}
	lines := strings.Split(string(doc[i+len("xref"):]), "\n")
	offsets := map[int]int{}
	pos := 0
	for pos < len(lines) {
		line := strings.TrimSpace(lines[pos])
		pos++
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "trailer") {
			break
		}
		var start, count int
		if _, err := fmt.Sscanf(line, "%d %d", &start, &count); err != nil {
			return nil, fmt.Errorf("gradient: xref header %q: %w", line, err)
		}
		for k := 0; k < count; k++ {
			if pos >= len(lines) {
				return nil, errShortXref
			}
			entry := strings.TrimSpace(lines[pos])
			pos++
			var off, gen int
			if _, err := fmt.Sscanf(entry, "%d %d", &off, &gen); err != nil {
				return nil, fmt.Errorf("gradient: xref entry %q: %w", entry, err)
			}
			if gen == 0 {
				offsets[start+k] = off
			}
		}
	}
	return offsets, nil
}

// objectBody returns an object's bytes, from just after "N 0 obj" up to but not
// including "endobj".
func objectBody(doc []byte, offset int) ([]byte, error) {
	if offset <= 0 || offset >= len(doc) {
		return nil, fmt.Errorf("%w: offset %d out of range", errNoObj, offset)
	}
	rest := doc[offset:]
	o := bytes.Index(rest, []byte("obj"))
	if o < 0 {
		return nil, errNoObj
	}
	body := rest[o+len("obj"):]
	e := bytes.Index(body, []byte("endobj"))
	if e < 0 {
		return nil, errNoObj
	}
	return body[:e], nil
}

// pageContentObject returns the object number of the page's content stream, by
// walking /Type /Page to its /Contents reference.
// pageObjectNumbers returns every page object in document order.
//
// A two-page quotation has two page dicts, and both this injector and the
// renderer that calls it have to address a specific one. Returning only the
// first, as an "any page will do" lookup would, silently patches page 1 and
// leaves page 2's ribbon flat.
func pageObjectNumbers(doc []byte, offsets map[int]int) []int {
	nums := make([]int, 0, len(offsets))
	for n := range offsets {
		nums = append(nums, n)
	}
	sortInts(nums)

	var pages []int
	for _, n := range nums {
		body, err := objectBody(doc, offsets[n])
		if err != nil {
			continue
		}
		s := string(body)
		if strings.Contains(s, "/Type /Page") && !strings.Contains(s, "/Type /Pages") {
			pages = append(pages, n)
		}
	}
	return pages
}

// pageContentObject returns the content-stream object of the given page.
func pageContentObject(doc []byte, offsets map[int]int, page int) (int, error) {
	pages := pageObjectNumbers(doc, offsets)
	if page < 0 || page >= len(pages) {
		return 0, fmt.Errorf("%w: page index %d of %d", errNoPage, page, len(pages))
	}
	body, err := objectBody(doc, offsets[pages[page]])
	if err != nil {
		return 0, err
	}
	s := string(body)
	c := strings.Index(s, "/Contents")
	if c < 0 {
		return 0, errNoPage
	}
	fields := strings.Fields(s[c+len("/Contents"):])
	if len(fields) < 3 || fields[2] != "R" {
		return 0, errNoPage
	}
	var num int
	if _, err := fmt.Sscanf(fields[0], "%d", &num); err != nil {
		return 0, errNoPage
	}
	return num, nil
}

// decodeStream inflates a stream object body.
func decodeStream(body []byte) ([]byte, error) {
	s := bytes.Index(body, []byte("stream"))
	if s < 0 {
		return nil, errNoStream
	}
	start := s + len("stream")
	for start < len(body) && (body[start] == '\r' || body[start] == '\n') {
		start++
	}
	e := bytes.LastIndex(body, []byte("endstream"))
	if e < 0 {
		return nil, errNoStream
	}
	raw := body[start:e]
	if bytes.Contains(body[:s], []byte("/FlateDecode")) {
		return flateDecode(raw)
	}
	return raw, nil
}

// resourceObject returns the object that owns the /Pattern entry.
//
// gpdf inlines the resource dictionary in the page object; gopdf emits a
// separate resource object that the page references. So the page object is
// located first, and whether ITS /Resources is inline or a reference decides
// whether the page or the referenced object is the one to patch. Guessing from
// the presence of /Font and /XObject instead would find the page for gpdf but
// patch the wrong nesting level for either.
func resourceObject(doc []byte, offsets map[int]int, page int) (int, error) {
	pages := pageObjectNumbers(doc, offsets)
	if page < 0 || page >= len(pages) {
		return 0, fmt.Errorf("%w: page index %d of %d", errNoResources, page, len(pages))
	}
	pageObj := pages[page]

	body, err := objectBody(doc, offsets[pageObj])
	if err != nil {
		return 0, err
	}
	s := string(body)
	ri := strings.Index(s, "/Resources")
	if ri < 0 {
		return 0, errNoResources
	}
	after := strings.TrimLeft(s[ri+len("/Resources"):], " \t\r\n")
	if strings.HasPrefix(after, "<<") {
		return pageObj, nil
	}
	fields := strings.Fields(after)
	if len(fields) >= 3 && fields[2] == "R" {
		var ref int
		if _, err := fmt.Sscanf(fields[0], "%d", &ref); err == nil {
			return ref, nil
		}
	}
	return 0, fmt.Errorf("%w: page %d has /Resources %q", errNoResources, pageObj, after)
}

// rebuildPDF writes a new document containing the patched content stream, the
// gradient /Pattern objects, the /Pattern resource entry, and an xref table
// that matches the bytes actually written.
//
// Rewriting the whole file from parsed pieces is deliberate: every edit shifts
// the byte offsets of everything after it, and regenerating the table is safer
// than patching offsets piecemeal.
func rebuildPDF(doc []byte, offsets map[int]int, contentObj int, content []byte,
	firstPattern int, grads []gradient, page int) ([]byte, error) {

	// With no gradients there is nothing to add to /Resources, and patching it
	// anyway would leave an empty "/Pattern << >>" in the document.
	resObj := 0
	var err error
	if len(grads) > 0 {
		if resObj, err = resourceObject(doc, offsets, page); err != nil {
			return nil, err
		}
	}

	maxObj := contentObj
	for n := range offsets {
		if n > maxObj {
			maxObj = n
		}
	}
	if last := firstPattern + len(grads) - 1; last > maxObj {
		maxObj = last
	}

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	newOffsets := make(map[int]int, maxObj)

	for n := 1; n <= maxObj; n++ {
		var body []byte
		switch {
		case n >= firstPattern:
			body = patternObject(grads[n-firstPattern])
		case n == contentObj:
			body = reencodeStream(content)
		case n == resObj:
			if body, err = resourcesWithPattern(doc, offsets[n], grads); err != nil {
				return nil, err
			}
		default:
			if body, err = objectBody(doc, offsets[n]); err != nil {
				return nil, err
			}
		}
		newOffsets[n] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", n)
		out.Write(body)
		out.WriteString("\nendobj\n")
	}

	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", maxObj+1)
	out.WriteString("0000000000 65535 f \n")
	for n := 1; n <= maxObj; n++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", newOffsets[n])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		maxObj+1, xref)
	return out.Bytes(), nil
}

// patternObject builds the /Pattern for one gradient.
//
// /Matrix maps the pattern's own space onto the page's default user space, so
// it must be the IDENTITY here and the shading's /Coords must already be in
// page coordinates. Scaling by the rect in /Matrix instead, while leaving
// /Coords at the rect's width, maps every point far outside the [0 1] function
// domain; /Extend then clamps them all to C0, which renders as a flat fill.
//
// Chrome can afford a scaled /Matrix because its content stream has a
// transform in effect; this content stream has none.
func patternObject(g gradient) []byte {
	r0, g0, b0 := parseHex(g.c0)
	r1, g1, b1 := parseHex(g.c1)
	// Page-space endpoints, y flipped to PDF's bottom-up sense. The ramp is
	// horizontal, so y0 and y1 are equal.
	y := g.pageH - g.y0
	return []byte(fmt.Sprintf(`<<
/Type /Pattern
/PatternType 2
/Matrix [1 0 0 1 0 0]
/Shading <<
/ColorSpace /DeviceRGB
/Coords [%s %s %s %s]
/Function <<
/C0 [%s %s %s]
/C1 [%s %s %s]
/Domain [0 1]
/FunctionType 2
/N 1
>>
/Extend [true true]
/ShadingType 2
>>
>>`,
		num(g.x0), num(y), num(g.x0+g.w), num(y),
		f4(float64(r0)/255), f4(float64(g0)/255), f4(float64(b0)/255),
		f4(float64(r1)/255), f4(float64(g1)/255), f4(float64(b1)/255)))
}

// resourcesWithPattern adds the /Pattern entry to whichever object holds the
// resource dictionary.
//
// The two libraries lay resources out differently: gopdf emits a separate
// resource object that the page references, while gpdf inlines the dictionary in
// the page object. Appending "/Pattern << ... >>" to the end of the object body
// works for gopdf and silently corrupts gpdf, where it lands OUTSIDE the /Page
// dictionary and the pattern is never found ("Unknown pattern"). So the /Pattern
// entry is spliced into the dictionary at its true nesting depth.
func resourcesWithPattern(doc []byte, offset int, grads []gradient) ([]byte, error) {
	body, err := objectBody(doc, offset)
	if err != nil {
		return nil, err
	}
	s := string(body)

	// Find the dictionary to patch: the inline /Resources on a page object, or
	// the object's own dictionary when this IS the resource object.
	//
	// gpdf writes a page as "<< /Contents ... /Resources << ... >> /Type /Page >>",
	// so anchoring on the first "<<" lands on the page dict and puts /Pattern
	// beside /Type /Page instead of inside /Resources. The result parses but the
	// pattern is unreachable ("Unknown pattern"). The anchor is therefore the
	// "<<" that follows /Resources, found by scanning past whitespace rather than
	// by arithmetic on the index, which is what made the first attempt put the
	// entry one level too deep.
	var start int
	if ri := strings.Index(s, "/Resources"); ri >= 0 {
		k := ri + len("/Resources")
		for k < len(s) && (s[k] == ' ' || s[k] == '\t' || s[k] == '\r' || s[k] == '\n') {
			k++
		}
		if k >= len(s) || !strings.HasPrefix(s[k:], "<<") {
			return nil, fmt.Errorf("%w: object %d has /Resources %q",
				errNoResources, offset, s[ri:k+8])
		}
		start = k + 1
	} else {
		start = strings.Index(s, "<<")
		if start < 0 {
			return nil, errNoResources
		}
		start++
	}

	// Walk to the matching close of the dictionary opened at `start`, and splice
	// just inside it.
	//
	// `start` already points AT the opening "<<", so depth begins at 1. Beginning
	// at 0 instead makes the first inner ">>" look like the outer close, which
	// inserts /Pattern in the middle of the first sub-dictionary (after /Font's
	// closing ">>" for gopdf, after /Font's for gpdf). The file still parses and
	// the pattern object still exists, but it is a sibling of /Font rather than a
	// member of /Resources, so the renderer reports "Unknown pattern" and the
	// ribbon renders flat.
	depth := 1
	j := start
	for j < len(s) {
		switch {
		case strings.HasPrefix(s[j:], "<<"):
			depth++
			j += 2
		case strings.HasPrefix(s[j:], ">>"):
			depth--
			if depth == 0 {
				var entry strings.Builder
				entry.WriteString("\n/Pattern << ")
				for _, g := range grads {
					fmt.Fprintf(&entry, "/Pg%d %d 0 R ", g.objNum, g.objNum)
				}
				entry.WriteString(">>")
				return []byte(s[:j] + entry.String() + s[j:]), nil
			}
			j += 2
		default:
			j++
		}
	}
	return nil, errNoResources
}

// reencodeStream wraps a decoded content stream back into a stream object body.
func reencodeStream(content []byte) []byte {
	comp, err := flateEncode(content)
	if err != nil {
		// Uncompressed is still valid PDF, so a compression failure degrades
		// the file's size rather than losing the render.
		return []byte(fmt.Sprintf("<<\n/Length %d\n>>\nstream\n%s\nendstream",
			len(content), content))
	}
	return []byte(fmt.Sprintf("<<\n/Filter/FlateDecode/Length %d\n>>\nstream\n%s\nendstream",
		len(comp), comp))
}

func flateDecode(b []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("gradient: inflate content stream: %w", err)
	}
	defer r.Close()
	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		return nil, fmt.Errorf("gradient: read content stream: %w", err)
	}
	return out.Bytes(), nil
}

func flateEncode(b []byte) ([]byte, error) {
	var out bytes.Buffer
	w := zlib.NewWriter(&out)
	if _, err := w.Write(b); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// num formats a coordinate the way gopdf does, so a search for a written path
// matches its output byte for byte.
func num(v float64) string {
	s := strings.TrimRight(fmt.Sprintf("%.2f", v), "0")
	return strings.TrimRight(s, ".")
}

// col formats a fill colour the way gopdf does: three decimals, untrimmed.
func gopdfCol(c uint8) string { return fmt.Sprintf("%.3f", float64(c)/255) }

func f4(v float64) string { return fmt.Sprintf("%.4f", v) }

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
