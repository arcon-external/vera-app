-- 047_session_expiry_renewal: a session now carries two deadlines and a
-- remembered flag, so the desktop can slide the idle deadline with a
-- "Stay signed in" renewal without ever crossing a hard cap.
--
--   expires_at            the idle deadline; renewal may push it forward
--   absolute_expires_at   the hard cap; no renewal may cross it
--   last_seen_at          when the session was last issued or renewed
--   remembered            which lifetime pair the session was issued under
--
-- Existing sessions are not silently extended: their absolute cap becomes the
-- fixed expiry they were stamped with under the old model, and their
-- remembered flag is inferred from that stamped lifetime (8 hours standard,
-- 30 days remembered). Their last_seen_at is their created_at because no
-- later use was ever recorded.
ALTER TABLE {{schema}}.sessions
    ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS absolute_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS remembered BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE {{schema}}.sessions
SET absolute_expires_at = expires_at
WHERE absolute_expires_at IS NULL;

UPDATE {{schema}}.sessions
SET last_seen_at = created_at
WHERE last_seen_at IS NULL;

UPDATE {{schema}}.sessions
SET remembered = TRUE
WHERE remembered = FALSE
  AND expires_at - created_at > INTERVAL '1 day';

ALTER TABLE {{schema}}.sessions
    ALTER COLUMN last_seen_at SET NOT NULL,
    ALTER COLUMN absolute_expires_at SET NOT NULL;
