package api

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"
)

// tinyPNG encodes a solid-color PNG of the given size.
func tinyPNG(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 60, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestUpdateDisplayName(t *testing.T) {
	server, _ := testServer(t, "vera_api_profile_name")
	token := doLogin(t, server, "api-admin@example.com", "test-password")

	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/auth/profile",
		`{"displayName":""}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("empty display name = %d; want 422", code)
	}
	long := strings.Repeat("a", 81)
	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/auth/profile",
		`{"displayName":"`+long+`"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("81-char display name = %d; want 422", code)
	}

	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/auth/profile",
		`{"displayName":"New Name"}`); code != http.StatusOK {
		t.Fatalf("update display name = %d; want 200", code)
	}
	code, me := doAdminRequest(t, server, token, http.MethodGet, "/v1/auth/me", "")
	if code != http.StatusOK {
		t.Fatalf("me = %d; want 200", code)
	}
	if me["displayName"] != "New Name" {
		t.Errorf("displayName = %v; want New Name", me["displayName"])
	}
}

func TestAvatarUploadRoundTrip(t *testing.T) {
	server, _ := testServer(t, "vera_api_profile_avatar")
	token := doLogin(t, server, "api-admin@example.com", "test-password")

	payload := `{"avatarBase64":"` + tinyPNG(t, 4, 4) + `","avatarMime":"image/png"}`
	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/auth/profile", payload); code != http.StatusOK {
		t.Fatalf("avatar upload = %d; want 200", code)
	}
	code, me := doAdminRequest(t, server, token, http.MethodGet, "/v1/auth/me", "")
	if code != http.StatusOK {
		t.Fatalf("me = %d; want 200", code)
	}
	avatar, _ := me["avatarUrl"].(string)
	if !strings.HasPrefix(avatar, "data:image/png;base64,") {
		t.Fatalf("avatarUrl = %.40q...; want data:image/png URL", avatar)
	}

	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/auth/profile",
		`{"removeAvatar":true}`); code != http.StatusOK {
		t.Fatalf("avatar remove = %d; want 200", code)
	}
	code, me = doAdminRequest(t, server, token, http.MethodGet, "/v1/auth/me", "")
	if code != http.StatusOK {
		t.Fatalf("me = %d; want 200", code)
	}
	if _, present := me["avatarUrl"]; present {
		t.Error("avatarUrl still present after removal")
	}
}

func TestAvatarRejectsBadUploads(t *testing.T) {
	server, _ := testServer(t, "vera_api_profile_reject")
	token := doLogin(t, server, "api-admin@example.com", "test-password")

	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/auth/profile",
		`{"avatarBase64":"`+base64.StdEncoding.EncodeToString([]byte("not an image"))+`","avatarMime":"image/png"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("non-image upload = %d; want 422", code)
	}

	big := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x61}, 512*1024+1))
	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/auth/profile",
		`{"avatarBase64":"`+big+`","avatarMime":"image/png"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("oversized upload = %d; want 422", code)
	}

	huge := tinyPNG(t, 2000, 2000)
	if code, _ := doAdminRequest(t, server, token, http.MethodPatch, "/v1/auth/profile",
		`{"avatarBase64":"`+huge+`","avatarMime":"image/png"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("oversized dimensions = %d; want 422", code)
	}
}
