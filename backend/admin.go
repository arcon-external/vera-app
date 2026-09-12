package backend

// ManagedUser is the admin-facing user shape. It never contains password
// hashes, session tokens, or invite tokens.
type ManagedUser struct {
	ID                 int64
	Email              string
	DisplayName        string
	Role               string
	IsActive           bool
	MustChangePassword bool
	LastLoginAt        string
}

// AdminAPI exposes user management to the frontend. Every method requires an
// admin session; the server enforces this independently.
type AdminAPI struct{}

// NewAdminAPI builds the frontend admin binding. The shared API client must
// be configured first via ConfigureAPI.
func NewAdminAPI() *AdminAPI {
	return &AdminAPI{}
}

func (a *AdminAPI) ListUsers() ([]ManagedUser, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Users []ManagedUser `json:"users"`
	}
	if err := client.Get("/v1/admin/users", &response); err != nil {
		return nil, err
	}
	if response.Users == nil {
		response.Users = []ManagedUser{}
	}
	return response.Users, nil
}

func (a *AdminAPI) CreateUser(email, displayName, role string) (ManagedUser, error) {
	client, err := api()
	if err != nil {
		return ManagedUser{}, err
	}
	var user ManagedUser
	if err := client.Post("/v1/admin/users", map[string]string{
		"email":       email,
		"displayName": displayName,
		"role":        role,
	}, &user); err != nil {
		return ManagedUser{}, err
	}
	return user, nil
}

func (a *AdminAPI) SetUserRole(id int64, role string) error {
	client, err := api()
	if err != nil {
		return err
	}
	return client.Patch("/v1/admin/users/"+itoa(id)+"/role", map[string]string{
		"role": role,
	}, nil)
}

func (a *AdminAPI) SetUserActive(id int64, active bool) error {
	client, err := api()
	if err != nil {
		return err
	}
	return client.Patch("/v1/admin/users/"+itoa(id)+"/active", map[string]bool{
		"active": active,
	}, nil)
}

func (a *AdminAPI) IssuePasswordReset(id int64) (string, error) {
	client, err := api()
	if err != nil {
		return "", err
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := client.Post("/v1/admin/users/"+itoa(id)+"/reset", nil, &response); err != nil {
		return "", err
	}
	return response.Token, nil
}

// ListDeletionRequests returns pending account-deletion requests.
func (a *AdminAPI) ListDeletionRequests() ([]DeletionRequest, error) {
	client, err := api()
	if err != nil {
		return nil, err
	}
	var response struct {
		Requests []DeletionRequest `json:"requests"`
	}
	if err := client.Get("/v1/admin/deletion-requests", &response); err != nil {
		return nil, err
	}
	if response.Requests == nil {
		response.Requests = []DeletionRequest{}
	}
	return response.Requests, nil
}

// DecideDeletion approves (soft-delete) or rejects a deletion request.
func (a *AdminAPI) DecideDeletion(id int64, approve bool) error {
	client, err := api()
	if err != nil {
		return err
	}
	action := "reject"
	if approve {
		action = "approve"
	}
	return client.Post("/v1/admin/deletion-requests/"+itoa(id)+"/"+action, nil, nil)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}
