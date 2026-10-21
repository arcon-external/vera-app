// Issue #478: synthetic scanned PDFs for tests.
//
// The scan tests start from the repository's own text PDFs, rasterise them with
// the pinned Poppler in the extractor image, and wrap the result back into a
// PDF whose page content is only an image. That gives a real "scanned" fixture
// with known text, without committing a customer document or a binary fixture.
//
// This file is test support: one full-page image per page, an optional
// rotation (to make a skewed scan), the default A4 page size, and Flate for the
// image data. It is not a production renderer.
package pdfgen

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"math"
)

// ImagePage is one raster page. RGB holds three bytes per pixel, row-major,
// top-to-bottom, and must be Width*Height*3 bytes long.
type ImagePage struct {
	Width         int
	Height        int
	RGB           []byte
	RotateDegrees float64
}

// NewImagePage copies an image into the RGB form an image-only PDF page needs.
func NewImagePage(source image.Image, rotateDegrees float64) ImagePage {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	rgb := make([]byte, width*height*3)
	index := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			red, green, blue, _ := source.At(x, y).RGBA()
			rgb[index] = byte(red >> 8)
			rgb[index+1] = byte(green >> 8)
			rgb[index+2] = byte(blue >> 8)
			index += 3
		}
	}
	return ImagePage{Width: width, Height: height, RGB: rgb, RotateDegrees: rotateDegrees}
}

// BuildImagePDF renders pages whose only content is one image each.
func BuildImagePDF(pages []ImagePage) []byte {
	converted := make([]Page, len(pages))
	for index := range pages {
		image := pages[index]
		converted[index] = Page{Image: &image}
	}
	return BuildMixed(converted)
}

func imageStream(page ImagePage) []byte {
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, _ = writer.Write(page.RGB)
	_ = writer.Close()
	header := fmt.Sprintf(
		"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB "+
			"/BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n",
		page.Width, page.Height, compressed.Len())
	var object bytes.Buffer
	object.WriteString(header)
	object.Write(compressed.Bytes())
	object.WriteString("\nendstream")
	return object.Bytes()
}

func pageDictionaryWithImage(imageObject, contentObject int) []byte {
	return []byte(fmt.Sprintf(
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] "+
			"/Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>",
		pageWidth, pageHeight, imageObject, contentObject))
}

// imageContent scales the image to the page and, when asked, rotates it about
// the page centre. A few degrees is enough to produce a skewed scan without
// pushing content off the page.
func imageContent(page ImagePage) string {
	radians := page.RotateDegrees * math.Pi / 180
	cosine, sine := math.Cos(radians), math.Sin(radians)
	centerX, centerY := pageWidth/2, pageHeight/2
	return fmt.Sprintf(
		"q\n1 0 0 1 %.4f %.4f cm\n%.6f %.6f %.6f %.6f 0 0 cm\n"+
			"1 0 0 1 %.4f %.4f cm\n%.4f 0 0 %.4f 0 0 cm\n/Im0 Do\nQ\n",
		centerX, centerY, cosine, sine, -sine, cosine, -centerX, -centerY,
		pageWidth, pageHeight)
}
