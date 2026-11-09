package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Ticket #536: a session now has two deadlines. expires_at is the idle
// deadline the renewal endpoint may push forward; absolute_expires_at is the
// hard cap that no renewal may cross. These tests pin the store half of that
// contract: what issuance stamps, what renewal extends, and which typed
// SessionError an ended session answers with.

func sessionInfo(t *testing.T, database *Store, tokenHash string, now time.Time) SessionInfo {
	t.Helper()
	_, info, err := database.GetSession(context.Background(), tokenHash, now)
	if err != nil {
		t.Fatalf("GetSession(%q): %v", tokenHash, err)
	}
	return info
}

func wantSessionCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a %s session error, got nil", code)
	}
	var sessionErr *SessionError
	if !errors.As(err, &sessionErr) {
		t.Fatalf("error %v is not a *SessionError", err)
	}
	if sessionErr.Code != code {
		t.Errorf("code = %q; want %q", sessionErr.Code, code)
	}
}

func TestCreateSessionStampsTheIdleAndAbsoluteDeadlines(t *testing.T) {
	database := openTestSchema(t, "vera_test_session_create")
	ctx := context.Background()
	user, err := database.CreateUser(ctx, "expiry@example.com", "Expiry", "test-hash", "staff")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	idle := now.Add(8 * time.Hour)
	absolute := now.Add(7 * 24 * time.Hour)
	if err := database.CreateSession(ctx, user.ID, "create-token", idle, absolute, false); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	info := sessionInfo(t, database, "create-token", now)
	if !info.ExpiresAt.Equal(idle) {
		t.Errorf("ExpiresAt = %s; want the idle deadline %s", info.ExpiresAt, idle)
	}
	if !info.AbsoluteExpiresAt.Equal(absolute) {
		t.Errorf("AbsoluteExpiresAt = %s; want %s", info.AbsoluteExpiresAt, absolute)
	}
	if info.Remembered {
		t.Error("Remembered = true; want false for a standard session")
	}
	if !info.CanRenew(now) {
		t.Error("CanRenew = false; a fresh session must be renewable")
	}

	rememberedIdle := now.Add(30 * 24 * time.Hour)
	rememberedAbsolute := now.Add(90 * 24 * time.Hour)
	if err := database.CreateSession(ctx, user.ID, "remembered-token", rememberedIdle, rememberedAbsolute, true); err != nil {
		t.Fatalf("CreateSession (remembered): %v", err)
	}
	remembered := sessionInfo(t, database, "remembered-token", now)
	if !remembered.Remembered {
		t.Error("Remembered = false; want true")
	}
	if !remembered.AbsoluteExpiresAt.Equal(rememberedAbsolute) {
		t.Errorf("remembered AbsoluteExpiresAt = %s; want %s", remembered.AbsoluteExpiresAt, rememberedAbsolute)
	}
}

func TestRenewSessionExtendsTheIdleDeadlineUpToTheAbsoluteCap(t *testing.T) {
	database := openTestSchema(t, "vera_test_session_renew")
	ctx := context.Background()
	user, err := database.CreateUser(ctx, "renew@example.com", "Renew", "test-hash", "staff")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	idle := now.Add(8 * time.Hour)
	absolute := now.Add(7 * 24 * time.Hour)
	if err := database.CreateSession(ctx, user.ID, "renew-token", idle, absolute, false); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// An hour of work: the idle deadline slides to now+1h+8h, still below the
	// cap, so the cap does not clamp it.
	first := now.Add(time.Hour)
	renewed, err := database.RenewSession(ctx, "renew-token", first, 8*time.Hour)
	if err != nil {
		t.Fatalf("RenewSession: %v", err)
	}
	if want := first.Add(8 * time.Hour); !renewed.ExpiresAt.Equal(want) {
		t.Errorf("expires_at = %s; want %s", renewed.ExpiresAt, want)
	}
	if !renewed.AbsoluteExpiresAt.Equal(absolute) {
		t.Errorf("absolute_expires_at = %s; renewal moved the cap; want %s", renewed.AbsoluteExpiresAt, absolute)
	}
	if !renewed.CanRenew(first) {
		t.Error("CanRenew = false while the idle deadline is below the cap")
	}

	// A session an hour from its cap: renewal has a full idle lifetime to
	// add but the cap clamps it, so the deadline lands exactly on the cap and
	// no further renewal is possible.
	nearCapIdle := absolute.Add(-time.Hour)
	if err := database.CreateSession(ctx, user.ID, "near-cap-token", nearCapIdle, absolute, false); err != nil {
		t.Fatalf("CreateSession near the cap: %v", err)
	}
	capped, err := database.RenewSession(ctx, "near-cap-token", absolute.Add(-4*time.Hour), 8*time.Hour)
	if err != nil {
		t.Fatalf("RenewSession near the cap: %v", err)
	}
	if !capped.ExpiresAt.Equal(absolute) {
		t.Errorf("expires_at = %s; want the cap %s", capped.ExpiresAt, absolute)
	}
	if capped.CanRenew(absolute.Add(-4 * time.Hour)) {
		t.Error("CanRenew = true at the absolute cap; renewal would extend nothing")
	}

	// At the cap the session is over, and renewal answers the typed expiry.
	_, err = database.RenewSession(ctx, "renew-token", absolute, 8*time.Hour)
	wantSessionCode(t, err, SessionCodeExpired)
}

func TestRenewSessionRefusesEndedSessions(t *testing.T) {
	database := openTestSchema(t, "vera_test_session_renew_refused")
	ctx := context.Background()
	user, err := database.CreateUser(ctx, "refused@example.com", "Refused", "test-hash", "staff")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := database.CreateSession(ctx, user.ID, "expired-idle",
		now.Add(-time.Hour), now.Add(24*time.Hour), false); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateSession(ctx, user.ID, "absolute-exhausted",
		now.Add(24*time.Hour), now.Add(-time.Hour), false); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateSession(ctx, user.ID, "revoked",
		now.Add(8*time.Hour), now.Add(7*24*time.Hour), false); err != nil {
		t.Fatal(err)
	}
	if err := database.RevokeSession(ctx, "revoked"); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		token string
		code  string
	}{
		{"idle deadline passed", "expired-idle", SessionCodeExpired},
		{"absolute cap passed", "absolute-exhausted", SessionCodeExpired},
		{"revoked", "revoked", SessionCodeRevoked},
		{"unknown token", "never-issued", SessionCodeInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := database.RenewSession(ctx, tc.token, now, 8*time.Hour)
			wantSessionCode(t, err, tc.code)
		})
	}
}

// TestRenewSessionRefusesAnEndBetweenReadAndWrite proves the UPDATE's WHERE
// clause, not just the read path. The session row is locked, the renewal's
// read completes and its UPDATE blocks; the transaction then expires the row
// and commits. Without the WHERE guards the blocked UPDATE would slide an
// expired session forward and report success.
func TestRenewSessionRefusesAnEndBetweenReadAndWrite(t *testing.T) {
	database := openTestSchema(t, "vera_test_session_renew_race")
	ctx := context.Background()
	user, err := database.CreateUser(ctx, "race@example.com", "Race", "test-hash", "staff")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	idle := now.Add(8 * time.Hour)
	if err := database.CreateSession(ctx, user.ID, "race-token", idle, now.Add(7*24*time.Hour), false); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	tx, err := database.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	var locked string
	if err := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT token_hash FROM %s.sessions WHERE token_hash = $1 FOR UPDATE`,
		database.configSchema()), "race-token").Scan(&locked); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, renewErr := database.RenewSession(ctx, "race-token", now, 8*time.Hour)
		done <- renewErr
	}()

	// The read is a plain MVCC read and does not wait on the lock; the UPDATE
	// does. Half a second is far longer than the read takes locally, so by now
	// the renewal is parked on the row lock.
	time.Sleep(500 * time.Millisecond)

	expired := now.Add(-time.Minute)
	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.sessions SET expires_at = $2 WHERE token_hash = $1`,
		database.configSchema()), "race-token", expired); err != nil {
		t.Fatalf("expire session: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	wantSessionCode(t, <-done, SessionCodeExpired)

	var stored time.Time
	if err := database.pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT expires_at FROM %s.sessions WHERE token_hash = $1`,
		database.configSchema()), "race-token").Scan(&stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !stored.Equal(expired) {
		t.Errorf("expires_at = %s after the refused renewal; want the expired value %s", stored, expired)
	}
}

func TestRenewSessionRefusesAnInactiveAccount(t *testing.T) {
	database := openTestSchema(t, "vera_test_session_renew_inactive")
	ctx := context.Background()
	user, err := database.CreateUser(ctx, "inactive@example.com", "Inactive", "test-hash", "staff")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := database.CreateSession(ctx, user.ID, "inactive-token",
		now.Add(8*time.Hour), now.Add(7*24*time.Hour), false); err != nil {
		t.Fatal(err)
	}
	if err := database.SetUserActive(ctx, user.ID, false); err != nil {
		t.Fatal(err)
	}
	_, err = database.RenewSession(ctx, "inactive-token", now, 8*time.Hour)
	wantSessionCode(t, err, SessionCodeInactive)
}

// TestSessionExpiryHonoursTheAbsoluteCap is the read path: a session whose
// idle deadline is still in the future is nevertheless over once the cap has
// passed, so no route may keep answering as if it were live.
func TestSessionExpiryHonoursTheAbsoluteCap(t *testing.T) {
	database := openTestSchema(t, "vera_test_session_absolute_read")
	ctx := context.Background()
	user, err := database.CreateUser(ctx, "cap@example.com", "Cap", "test-hash", "staff")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := database.CreateSession(ctx, user.ID, "capped-token",
		now.Add(30*24*time.Hour), now.Add(time.Hour), false); err != nil {
		t.Fatal(err)
	}

	if _, err := database.GetSessionUser(ctx, "capped-token", now.Add(30*time.Minute)); err != nil {
		t.Fatalf("GetSessionUser before the cap: %v", err)
	}
	_, err = database.GetSessionUser(ctx, "capped-token", now.Add(2*time.Hour))
	wantSessionCode(t, err, SessionCodeExpired)
}

// TestMigration047BackfillsExistingSessions pins the migration's promise:
// sessions issued before the sliding model keep the deadline they were given
// and are not silently extended. Their remembered flag is inferred from the
// lifetime they were stamped with, because the column did not exist when they
// were created.
func TestMigration047BackfillsExistingSessions(t *testing.T) {
	schema := "vera_test_session_migration"
	resetTestSchema(t, testDSN(t), schema)
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, testDSN(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	quoted := `"` + schema + `"`
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s.schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`, quoted)); err != nil {
		t.Fatalf("create migration table: %v", err)
	}
	migrations, err := loadMigrations(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		if migration.version >= 47 {
			continue
		}
		if _, err := pool.Exec(ctx, migration.sql); err != nil {
			t.Fatalf("apply migration %d: %v", migration.version, err)
		}
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			"INSERT INTO %s.schema_migrations (version, name) VALUES ($1, $2)",
			quoted), migration.version, migration.name); err != nil {
			t.Fatalf("record migration %d: %v", migration.version, err)
		}
	}

	created := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	var userID int64
	if err := pool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO %s.users (email, display_name, password_hash, role)
		VALUES ('legacy@example.com', 'Legacy', 'test-hash', 'staff')
		RETURNING id`, quoted)).Scan(&userID); err != nil {
		t.Fatalf("insert legacy user: %v", err)
	}
	for _, row := range []struct {
		token   string
		expires time.Time
	}{
		{"legacy-standard", created.Add(8 * time.Hour)},
		{"legacy-remembered", created.Add(30 * 24 * time.Hour)},
	} {
		if _, err := pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.sessions (user_id, token_hash, expires_at, created_at)
			VALUES ($1, $2, $3, $4)`, quoted),
			userID, row.token, row.expires, created); err != nil {
			t.Fatalf("insert %s: %v", row.token, err)
		}
	}

	// Opening the store applies 047 over the legacy rows.
	database, err := Open(ctx, Config{DSN: testDSN(t), Schema: schema})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(database.Close)

	for _, tc := range []struct {
		token      string
		expires    time.Time
		remembered bool
	}{
		{"legacy-standard", created.Add(8 * time.Hour), false},
		{"legacy-remembered", created.Add(30 * 24 * time.Hour), true},
	} {
		var expiresAt, absoluteExpiresAt, lastSeenAt time.Time
		var remembered bool
		if err := pool.QueryRow(ctx, fmt.Sprintf(`
			SELECT expires_at, absolute_expires_at, last_seen_at, remembered
			FROM %s.sessions WHERE token_hash = $1`, quoted), tc.token).Scan(
			&expiresAt, &absoluteExpiresAt, &lastSeenAt, &remembered); err != nil {
			t.Fatalf("read %s: %v", tc.token, err)
		}
		if !absoluteExpiresAt.Equal(tc.expires) {
			t.Errorf("%s absolute_expires_at = %s; want the stamped expiry %s (no silent extension)",
				tc.token, absoluteExpiresAt, tc.expires)
		}
		if !expiresAt.Equal(tc.expires) {
			t.Errorf("%s expires_at = %s; want %s", tc.token, expiresAt, tc.expires)
		}
		if remembered != tc.remembered {
			t.Errorf("%s remembered = %v; want %v", tc.token, remembered, tc.remembered)
		}
		if !lastSeenAt.Equal(created) {
			t.Errorf("%s last_seen_at = %s; want created_at %s", tc.token, lastSeenAt, created)
		}
		info := SessionInfo{ExpiresAt: expiresAt, AbsoluteExpiresAt: absoluteExpiresAt, Remembered: remembered}
		if info.CanRenew(created.Add(time.Hour)) {
			t.Errorf("%s CanRenew = true; a legacy session must not gain a renewal window it never had", tc.token)
		}
	}
}
