-- 022_company_currency: one currency per company, from the shipped catalogue
-- (see backend/currency). TEXT with a default, exactly like the theme columns
-- in 014: IDR is the default, so every profile written before this column
-- reads back as what it always was, and stored integers (whole rupiah for
-- IDR's exponent 0) are unchanged.
--
-- Validation lives at the store boundary (checkCurrency names the field);
-- reads normalize again, so an unknown stored code renders as IDR rather
-- than breaking a document.
ALTER TABLE {{schema}}.company_profile
    ADD COLUMN IF NOT EXISTS currency TEXT NOT NULL DEFAULT 'IDR';
