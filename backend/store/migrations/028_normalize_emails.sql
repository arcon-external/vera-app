-- Existing login identities use the same canonical form as new writes.
UPDATE {{schema}}.users
SET email = LOWER(TRIM(email))
WHERE email <> LOWER(TRIM(email));
