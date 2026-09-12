package api

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"golang.org/x/image/webp"

	"vera-app/backend/store"
)

const (
	maxAvatarBytes = 512 * 1024
	maxAvatarDim   = 1024
)

var allowedAvatarMimes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

func init() {
	image.RegisterFormat("webp", "RIFF????WEBP", webp.Decode, webp.DecodeConfig)
}

// avatarURL renders stored avatar bytes as a data URL, or "" when absent.
func avatarURL(user store.AuthUser) string {
	if len(user.Avatar) == 0 || user.AvatarMime == "" {
		return ""
	}
	return "data:" + user.AvatarMime + ";base64," + base64.StdEncoding.EncodeToString(user.Avatar)
}

// validateAvatar decodes and bounds-checks an uploaded picture.
func validateAvatar(data []byte, mime string) error {
	if len(data) == 0 {
		return fmt.Errorf("avatar is empty")
	}
	if len(data) > maxAvatarBytes {
		return fmt.Errorf("avatar exceeds 512KB")
	}
	if !allowedAvatarMimes[mime] {
		return fmt.Errorf("avatar must be PNG, JPEG, GIF, or WebP")
	}
	if detected := http.DetectContentType(data); !allowedAvatarMimes[detected] {
		return fmt.Errorf("avatar content is not a supported image")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("avatar is not a decodable image")
	}
	if config.Width > maxAvatarDim || config.Height > maxAvatarDim {
		return fmt.Errorf("avatar exceeds 1024px on a side")
	}
	return nil
}

func (s *Server) updateProfile(context echo.Context) error {
	me, _ := context.Get("user").(store.AuthUser)
	var request struct {
		DisplayName  string `json:"displayName"`
		AvatarBase64 string `json:"avatarBase64"`
		AvatarMime   string `json:"avatarMime"`
		RemoveAvatar bool   `json:"removeAvatar"`
	}
	if err := context.Bind(&request); err != nil {
		return context.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	name := strings.TrimSpace(request.DisplayName)
	if request.DisplayName != "" && (name == "" || len([]rune(name)) > 80) {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "display name must be 1–80 characters"})
	}
	if request.DisplayName == "" && request.AvatarBase64 == "" && !request.RemoveAvatar {
		return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "nothing to update"})
	}

	var avatar []byte
	mime := ""
	clearAvatar := request.RemoveAvatar
	if request.AvatarBase64 != "" {
		data, err := base64.StdEncoding.DecodeString(request.AvatarBase64)
		if err != nil {
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": "avatar is not valid base64"})
		}
		if err := validateAvatar(data, request.AvatarMime); err != nil {
			return context.JSON(http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		}
		avatar, mime = data, request.AvatarMime
		clearAvatar = false
	}

	if err := s.database.SetProfile(context.Request().Context(), me.ID, name, avatar, mime, clearAvatar); err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "update profile"})
	}
	updated, err := s.database.GetUserByID(context.Request().Context(), me.ID)
	if err != nil {
		return context.JSON(http.StatusInternalServerError, map[string]string{"error": "reload profile"})
	}
	response := map[string]any{
		"displayName": updated.DisplayName,
	}
	if url := avatarURL(updated); url != "" {
		response["avatarUrl"] = url
	}
	return context.JSON(http.StatusOK, response)
}
