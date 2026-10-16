package apiclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// Client talks to veraserver over TLS. The zero token provider means
// unauthenticated (used for login and health checks).
type Client struct {
	token          func() string
	http           *http.Client
	OnUnauthorized func()
	baseURL        string
}

// StatusError is a response the server refused: anything outside 2xx, and the
// 401 that ends a session.
//
// It carries the status code because callers want different things from it. The
// desktop shows Message and nothing else. The CLI maps the code to an exit
// status, so a script can branch on "not found" without matching English.
type StatusError struct {
	StatusCode int
	Message    string
}

func (e *StatusError) Error() string { return e.Message }

// MarshalJSON keeps the shape a plain message. The desktop reads `message` off
// whatever Wails hands it (see getErrorMessage in App.tsx), and a typed error
// must not arrive there as an object it cannot read.
func (e *StatusError) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Message string `json:"message"`
	}{Message: e.Message})
}

// StatusOf reports the HTTP status behind an error, or 0 when the error did not
// come from a response at all (an unreachable server, a malformed URL).
func StatusOf(err error) int {
	var status *StatusError
	if errors.As(err, &status) {
		return status.StatusCode
	}
	return 0
}

// BaseURL reports the API address this client talks to. The verification
// portal is served by the same service, so documents printed by this client
// link back to it.
func (c *Client) BaseURL() string {
	return strings.TrimRight(c.baseURL, "/")
}

// SetTimeout bounds each request. The desktop keeps the 30s default; the CLI
// lets the caller choose, because a bulk export and a status check want
// different patience.
func (c *Client) SetTimeout(d time.Duration) {
	if d > 0 {
		c.http.Timeout = d
	}
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

// ValidateBaseURL rejects cleartext credentials sent to remote hosts.
func ValidateBaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%q is not a usable API URL", raw)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return nil
		}
		return fmt.Errorf("%s is plain http on a remote host; use https so passwords and tokens do not cross the network in clear", raw)
	}
	return fmt.Errorf("%q is not a usable HTTP(S) API URL", raw)
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
		return &StatusError{StatusCode: response.StatusCode, Message: apiError(response, "signed out; sign in again")}
	}
	if response.StatusCode == http.StatusNoContent {
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &StatusError{StatusCode: response.StatusCode, Message: apiError(response, "server request failed")}
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
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	if err != nil {
		return fallback
	}
	return apiErrorBody(body, fallback)
}

// PostForm sends an application/x-www-form-urlencoded body and returns the raw
// response body.
//
// The public verification portal is the one endpoint that is neither JSON in nor
// JSON out: it takes the same form a browser would submit and answers with the
// page it renders. Keeping that shape here means the CLI and any test go through
// the identical path a customer does, rate limiter included.
func (c *Client) PostForm(path string, form map[string]string) (string, error) {
	values := url.Values{}
	for key, value := range form {
		values.Set(key, value)
	}
	request, err := http.NewRequest(http.MethodPost, c.baseURL+path, strings.NewReader(values.Encode()))
	if err != nil {
		return "", fmt.Errorf("build form request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if token := c.token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return "", fmt.Errorf("reach the quotation server: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read server response: %w", err)
	}
	if response.StatusCode == http.StatusUnauthorized {
		if c.OnUnauthorized != nil {
			c.OnUnauthorized()
		}
		return "", &StatusError{StatusCode: response.StatusCode, Message: apiErrorBody(body, "signed out; sign in again")}
	}
	// 404 is a real answer from the portal -- "no document matches that code" --
	// and its page still has to be read, so only 5xx counts as a failure here.
	if response.StatusCode >= 500 {
		return "", &StatusError{StatusCode: response.StatusCode, Message: apiErrorBody(body, "server request failed")}
	}
	return string(body), nil
}

// GetBody sends a GET and returns the raw response body.
//
// The admin portal preview is the other endpoint that is neither JSON in nor
// JSON out: it answers with the verification page itself, as HTML for the
// settings iframe. A JSON decoder would mangle that, so the body comes back
// whole, exactly as PostForm does for the public portal.
func (c *Client) GetBody(path string) (string, error) {
	request, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return "", fmt.Errorf("build API request: %w", err)
	}
	if token := c.token(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return "", fmt.Errorf("reach the quotation server: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read server response: %w", err)
	}
	if response.StatusCode == http.StatusUnauthorized {
		if c.OnUnauthorized != nil {
			c.OnUnauthorized()
		}
		return "", &StatusError{StatusCode: response.StatusCode, Message: apiErrorBody(body, "signed out; sign in again")}
	}
	if response.StatusCode != http.StatusOK {
		return "", &StatusError{StatusCode: response.StatusCode, Message: apiErrorBody(body, "server request failed")}
	}
	return string(body), nil
}

// apiErrorBody is apiError for a body already read.
func apiErrorBody(body []byte, fallback string) string {
	var problem struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &problem) != nil || problem.Error == "" {
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
		return &StatusError{StatusCode: response.StatusCode, Message: "signed out; sign in again"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &StatusError{StatusCode: response.StatusCode, Message: apiError(response, "server request failed")}
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
		return "", nil, &StatusError{StatusCode: response.StatusCode, Message: "signed out; sign in again"}
	}
	if response.StatusCode != http.StatusOK {
		return "", nil, &StatusError{StatusCode: response.StatusCode, Message: apiError(response, "server request failed")}
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
