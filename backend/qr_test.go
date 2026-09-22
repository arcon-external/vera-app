package backend

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"testing"
)

func TestBuildVerificationQREncodesAPNG(t *testing.T) {
	url := "https://vera-api-171860943410.asia-southeast1.run.app/v/9f3a4c2e1b7d4a8e"
	encoded, err := BuildVerificationQR(url)
	if err != nil {
		t.Fatalf("build QR: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if img.Bounds().Dx() != verificationQRSize || img.Bounds().Dy() != verificationQRSize {
		t.Errorf("QR = %v; want a %dpx square", img.Bounds().Size(), verificationQRSize)
	}
}

// TestVerificationQRIsDeterministic guards the property the signature and
// print workflow depend on: the same URL always produces the same image, and
// a different URL always produces a different one.
func TestVerificationQRIsDeterministic(t *testing.T) {
	url := "https://example.test/v/5b3e70e474604500"
	first, err := BuildVerificationQR(url)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := BuildVerificationQR(url)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first != second {
		t.Error("same URL produced different QR images")
	}
	other, err := BuildVerificationQR("https://example.test/v/5b3e70e474604501")
	if err != nil {
		t.Fatalf("other: %v", err)
	}
	if first == other {
		t.Error("different URLs produced identical QR images")
	}
}

func TestBuildVerificationQRRejectsEmptyURL(t *testing.T) {
	if _, err := BuildVerificationQR(""); err == nil {
		t.Error("empty URL accepted; want an error")
	}
}
