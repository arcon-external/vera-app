package backend

import (
	"encoding/base64"
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

// verificationQRSize is the rendered pixel width. Printing scales it down to
// roughly 110px, so generating larger keeps module edges crisp in the PDF.
const verificationQRSize = 320

// BuildVerificationQR renders a PNG QR code for a verification URL and
// returns it base64-encoded for inlining in the document template — the same
// delivery channel the logo and other assets already use, so the printed
// document needs no network access to render.
//
// Error correction is Medium: enough redundancy to survive a photocopy or a
// slightly small print without inflating the code past the layout slot.
func BuildVerificationQR(url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("verification URL is required")
	}
	png, err := qrcode.Encode(url, qrcode.Medium, verificationQRSize)
	if err != nil {
		return "", fmt.Errorf("encode verification QR: %w", err)
	}
	return base64.StdEncoding.EncodeToString(png), nil
}
