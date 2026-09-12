package backend

import (
	"fmt"

	"vera-app/backend/apiclient"
)

// apiClient is the shared veraserver client, configured once in main.go.
// Tests replace it with a stub pointing at httptest servers.
var apiClient *apiclient.Client

// ConfigureAPI sets the shared API client. It must be called before any
// network-backed method runs.
func ConfigureAPI(client *apiclient.Client) {
	apiClient = client
}

func api() (*apiclient.Client, error) {
	if apiClient == nil {
		return nil, fmt.Errorf("API client is not configured")
	}
	return apiClient, nil
}
