// Package pdfgen builds small synthetic PDFs for tests and manual verification.
//
// It exists so the extraction tests can use a real PDF with known words and
// positions without committing a customer document. The generator writes a
// classic-xref, single-font document; Poppler reads it as ordinary text. This
// is test support, not a production renderer: it supports the Helvetica
// base-14 face, ASCII text, multi-page tables and full-page image pages (for
// synthetic scans), which is all the fixtures need.
package pdfgen

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	pageWidth  = 595.0
	pageHeight = 842.0
	fontSize   = 10.0
)

// Line is one text run at a position measured from the page's top-left, in PDF
// points. Size zero means 10pt.
type Line struct {
	Text string
	X    float64
	Y    float64
	Size float64
}

// Page is one synthetic page: either text lines or one full-page image.
// Exactly one of Lines and Image is used; an image page has no text layer, so
// it is the fixture for a scan.
type Page struct {
	Lines []Line
	Image *ImagePage
}

// Build renders pages of lines into a valid PDF.
func Build(pages [][]Line) []byte {
	converted := make([]Page, len(pages))
	for index, lines := range pages {
		converted[index] = Page{Lines: lines}
	}
	return BuildMixed(converted)
}

// BuildMixed renders text pages and image-only pages into one PDF, which is
// what a mixed text-header/scanned-table fixture needs.
func BuildMixed(pages []Page) []byte {
	objects := make([][]byte, 0, 3+len(pages)*3)
	objects = append(objects,
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		nil, // page tree, filled once the page object numbers are known
		[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>"),
	)
	pageObjects := make([]int, 0, len(pages))
	for _, page := range pages {
		if page.Image != nil {
			imageObject := len(objects) + 1
			objects = append(objects, imageStream(*page.Image))
			contentObject := len(objects) + 1
			objects = append(objects, contentStreamBytes(imageContent(*page.Image)))
			pageObject := len(objects) + 1
			objects = append(objects, pageDictionaryWithImage(imageObject, contentObject))
			pageObjects = append(pageObjects, pageObject)
			continue
		}
		contentObject := len(objects) + 1
		objects = append(objects, contentStreamBytes(textContentStream(page.Lines)))
		pageObject := len(objects) + 1
		objects = append(objects, pageDictionaryWithFont(contentObject))
		pageObjects = append(pageObjects, pageObject)
	}
	kids := make([]string, 0, len(pageObjects))
	for _, number := range pageObjects {
		kids = append(kids, fmt.Sprintf("%d 0 R", number))
	}
	objects[1] = []byte(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>",
		strings.Join(kids, " "), len(pageObjects)))

	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects))
	for index, object := range objects {
		offsets = append(offsets, buffer.Len())
		fmt.Fprintf(&buffer, "%d 0 obj\n", index+1)
		buffer.Write(object)
		buffer.WriteString("\nendobj\n")
	}
	xref := buffer.Len()
	fmt.Fprintf(&buffer, "xref\n0 %d\n", len(objects)+1)
	buffer.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets {
		fmt.Fprintf(&buffer, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buffer, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objects)+1, xref)
	return buffer.Bytes()
}

func textContentStream(lines []Line) string {
	var content strings.Builder
	content.WriteString("BT\n")
	for _, line := range lines {
		size := line.Size
		if size <= 0 {
			size = fontSize
		}
		y := pageHeight - line.Y - size
		fmt.Fprintf(&content, "/F1 %.2f Tf\n1 0 0 1 %.2f %.2f Tm\n(%s) Tj\n",
			size, line.X, y, escape(line.Text))
	}
	content.WriteString("ET\n")
	return content.String()
}

func contentStreamBytes(content string) []byte {
	return []byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
}

func pageDictionaryWithFont(contentObject int) []byte {
	return []byte(fmt.Sprintf(
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] "+
			"/Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>",
		pageWidth, pageHeight, contentObject))
}

func escape(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
	return replacer.Replace(text)
}
