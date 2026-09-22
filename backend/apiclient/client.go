package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Client talks to veraserver over TLS. The zero token provider means
// unauthenticated (used for login and health checks).
type Client struct {
	baseURL        string
	token          func() string
	http           *http.Client
	OnUnauthorized func()
}

// BaseURL reports the API address this client talks to. The verification
// portal is served by the same service, so documents printed by this client
// link back to it.
func (c *Client) BaseURL() string {
	return strings.TrimRight(c.baseURL, "/")
}

// New builds a client for a base URL like https://vera.example.com.
func New(baseURL string, token func() string) *Client {
	if token == nil {
		token = func() string { return "" }
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode API request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build API request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if token := c.token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("reach the quotation server: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized {
		if c.OnUnauthorized != nil {
			c.OnUnauthorized()
		}
		return fmt.Errorf("%s", apiError(response, "signed out; sign in again"))
	}
	if response.StatusCode == http.StatusNoContent {
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s", apiError(response, "server request failed"))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("read server response: %w", err)
	}
	return nil
}

func apiError(response *http.Response, fallback string) string {
	var problem struct {
		Error string `json:"error"`
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	if err != nil || json.Unmarshal(body, &problem) != nil || problem.Error == "" {
		return fallback
	}
	if len(problem.Error) > 200 {
		return problem.Error[:200]
	}
	return problem.Error
}

// Health checks server liveness without authentication.
func (c *Client) Health() error {
	return c.Get("/healthz", nil)
}

// Get performs an authenticated GET decoding JSON into out (may be nil).
func (c *Client) Get(path string, out any) error {
	return c.do(http.MethodGet, path, nil, out)
}

// Post performs an authenticated POST with an optional JSON body.
func (c *Client) Post(path string, body, out any) error {
	return c.do(http.MethodPost, path, body, out)
}

// Put performs an authenticated PUT with an optional JSON body.
func (c *Client) Put(path string, body, out any) error {
	return c.do(http.MethodPut, path, body, out)
}

// Patch performs an authenticated PATCH with an optional JSON body.
func (c *Client) Patch(path string, body, out any) error {
	return c.do(http.MethodPatch, path, body, out)
}

// Delete performs an authenticated DELETE.
func (c *Client) Delete(path string, out any) error {
	return c.do(http.MethodDelete, path, nil, out)
}

// Upload posts a multipart file field and decodes the JSON response into
// out (may be nil). It is used for admin backup restores.
func (c *Client) Upload(path, fieldName, filename string, data []byte, out any) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		return fmt.Errorf("build upload: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return fmt.Errorf("build upload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("build upload: %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, c.baseURL+path, &body)
	if err != nil {
		return fmt.Errorf("build API request: %w", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if token := c.token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("reach the quotation server: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		if c.OnUnauthorized != nil {
			c.OnUnauthorized()
		}
		return fmt.Errorf("signed out; sign in again")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s", apiError(response, "server request failed"))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("decode server response: %w", err)
	}
	return nil
}

// Download performs an authenticated GET returning raw bytes and the
// filename from Content-Disposition (falling back to defaultName).
func (c *Client) Download(path, defaultName string) (filename string, data []byte, err error) {
	request, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return "", nil, fmt.Errorf("build API request: %w", err)
	}
	if token := c.token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return "", nil, fmt.Errorf("reach the quotation server: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		if c.OnUnauthorized != nil {
			c.OnUnauthorized()
		}
		return "", nil, fmt.Errorf("signed out; sign in again")
	}
	if response.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("%s", apiError(response, "server request failed"))
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, 512<<20))
	if err != nil {
		return "", nil, fmt.Errorf("read download: %w", err)
	}
	filename = defaultName
	if _, params, err := mime.ParseMediaType(response.Header.Get("Content-Disposition")); err == nil {
		if name := params["filename"]; name != "" {
			filename = filepath.Base(name)
		}
	}
	return filename, data, nil
}
