-- 014_company_theme: brand colors for the quotation template (q_*) and the
-- verification portal (p_*), independently settable. Defaults carry today's
-- palette on both surfaces; the derived roles land within a few units per
-- channel of the literals they replaced (max 7), so existing renders shift
-- imperceptibly until an admin changes a value.
ALTER TABLE {{schema}}.company_profile
    ADD COLUMN IF NOT EXISTS theme_q_deep TEXT NOT NULL DEFAULT '#1A237E',
    ADD COLUMN IF NOT EXISTS theme_q_accent TEXT NOT NULL DEFAULT '#284AF3',
    ADD COLUMN IF NOT EXISTS theme_q_soft TEXT NOT NULL DEFAULT '#D6E4FF',
    ADD COLUMN IF NOT EXISTS theme_q_ink TEXT NOT NULL DEFAULT '#1a1a1a',
    ADD COLUMN IF NOT EXISTS theme_p_deep TEXT NOT NULL DEFAULT '#1A237E',
    ADD COLUMN IF NOT EXISTS theme_p_accent TEXT NOT NULL DEFAULT '#284AF3',
    ADD COLUMN IF NOT EXISTS theme_p_soft TEXT NOT NULL DEFAULT '#D6E4FF',
    ADD COLUMN IF NOT EXISTS theme_p_ink TEXT NOT NULL DEFAULT '#1a1a1a';
