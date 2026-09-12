package auth

import (
	"time"

	"vera-app/backend/store"
)

// NewStaticService returns a Service whose session checks always resolve to
// the given user (or error). It is a test seam so quotation logic tests do
// not need a live database session.
func NewStaticService(user store.AuthUser, err error) *Service {
	service := &Service{
		limiter: NewRateLimiter(5, time.Minute),
	}
	service.setStaticForTest(user, err)
	return service
}

// StaffTestService returns a static signed-in staff user for tests.
func StaffTestService() *Service {
	return NewStaticService(store.AuthUser{
		ID:       1,
		Email:    "staff@example.com",
		Role:     "staff",
		IsActive: true,
	}, nil)
}

// AdminTestService returns a static signed-in admin user for tests.
func AdminTestService() *Service {
	return NewStaticService(store.AuthUser{
		ID:       2,
		Email:    "admin@example.com",
		Role:     "admin",
		IsActive: true,
	}, nil)
}
