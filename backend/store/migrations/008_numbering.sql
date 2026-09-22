-- 008_numbering: UUID identity + per-customer yearly numbering.
--
-- Numbers become 15CCYYNNNN (plain digits): per-(customer, year) counter keys
-- starting at 1 every January, allocated in store/sequences.go. The old global
-- UNIQUE(quote_number) would forbid that, so it becomes a composite guard on
-- (customer_id, quote_number): duplicates inside one customer-year cycle are
-- unrepresentable, while different customers may hold identical strings.
-- The mock-era global 'quotation' counter key is dropped; fresh keys are
-- created on demand per customer-year.
ALTER TABLE {{schema}}.quotations
    ADD COLUMN IF NOT EXISTS uuid UUID NOT NULL DEFAULT gen_random_uuid();

ALTER TABLE {{schema}}.quotations DROP CONSTRAINT IF EXISTS quotations_quote_number_key;

ALTER TABLE {{schema}}.quotations
    ADD CONSTRAINT quotation_cycle_unique UNIQUE (customer_id, quote_number);

DELETE FROM {{schema}}.sequence_counters WHERE name = 'quotation';
