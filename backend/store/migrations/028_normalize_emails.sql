-- Existing login identities use the same canonical form as new writes.
UPDATE {{schema}}.users AS u
SET email = LOWER(TRIM(u.email))
WHERE u.email <> LOWER(TRIM(u.email))
  AND NOT EXISTS (
    SELECT 1 FROM {{schema}}.users AS other
    WHERE other.id <> u.id
      AND LOWER(TRIM(other.email)) = LOWER(TRIM(u.email))
  );
