package apiclient

import "testing"

func TestValidateBaseURL(t *testing.T) {
	for _, tc := range []struct {
		url     string
		allowed bool
	}{
		{"https://example.com", true},
		{"http://example.com", false},
		{"http://127.0.0.1:8080", true},
		{"http://localhost:8080", true},
		{"http://[::1]:8080", true},
		{"garbage", false},
		{"ftp://localhost", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			err := ValidateBaseURL(tc.url)
			if (err == nil) != tc.allowed {
				t.Errorf("ValidateBaseURL(%q) = %v; allowed = %v", tc.url, err, tc.allowed)
			}
		})
	}
}
