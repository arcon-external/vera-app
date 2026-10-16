package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDeleteStaleSessions(t *testing.T) {
	database := openTestSchema(t, "vera_test_session_cleanup")
	ctx := context.Background()
	user, err := database.CreateUser(ctx, "cleanup@example.com", "Cleanup", "test-hash", "staff")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	cutoff := now.Add(-30 * 24 * time.Hour)
	pool, err := database.poolOrError()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name    string
		expires time.Time
		revoked *time.Time
	}{
		{"fresh", now.Add(time.Hour), nil},
		{"expired-1d", now.Add(-24 * time.Hour), nil},
		{"expired-at-cutoff", cutoff, nil},
		{"expired-31d", now.Add(-31 * 24 * time.Hour), nil},
		// A revoked session may have an expiry far in the future.
		{"revoked-31d", now.Add(24 * time.Hour), timePtr(now.Add(-31 * 24 * time.Hour))},
		{"revoked-1d", now.Add(-31 * 24 * time.Hour), timePtr(now.Add(-24 * time.Hour))},
	} {
		_, err := pool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s.sessions (user_id, token_hash, expires_at, revoked_at)
			VALUES ($1, $2, $3, $4)`, database.configSchema()), user.ID, row.name, row.expires, row.revoked)
		if err != nil {
			t.Fatalf("insert %s: %v", row.name, err)
		}
	}
	deleted, err := database.DeleteStaleSessions(ctx, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Errorf("deleted %d; want 2", deleted)
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`SELECT token_hash FROM %s.sessions ORDER BY token_hash`, database.configSchema()))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var remaining []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			t.Fatal(err)
		}
		remaining = append(remaining, token)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"expired-1d", "expired-at-cutoff", "fresh", "revoked-1d"}
	if fmt.Sprint(remaining) != fmt.Sprint(want) {
		t.Errorf("remaining = %v; want %v", remaining, want)
	}
}

func timePtr(t time.Time) *time.Time { return &t }
