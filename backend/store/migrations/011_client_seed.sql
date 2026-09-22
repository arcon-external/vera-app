-- 011_client_seed: populate the client directory with the five legacy
-- clients (codes 02-06) carried by the retired hard-coded maps. Rows that
-- already exist (e.g. code 02 upserted by past exports, with live
-- quotations) are never overwritten; primary addresses backfill only where
-- none exists. Reruns are no-ops.
INSERT INTO {{schema}}.customers (customer_code, legal_name)
VALUES
    (2, 'PT. Infineon Technologies Batam'),
    (3, 'PT. H-Tech Oilfield Equipment, NOV XL Systems'),
    (4, 'PT. Datalec Global Solution'),
    (5, 'PT. Panasonic Industrial Devices Batam'),
    (6, 'PT. Surya Teknologi Batam')
ON CONFLICT (customer_code) DO NOTHING;

INSERT INTO {{schema}}.customer_addresses (customer_id, address, is_primary)
SELECT c.id, v.address, TRUE
FROM {{schema}}.customers c
JOIN (VALUES
    (2, 'Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam, Kepulauan Riau, Indonesia'),
    (3, 'Kecamatan Nongsa, Kota Batam, Kepulauan Riau, Indonesia'),
    (4, 'Ruko Pollux Habibie No.E16, Kota Batam, Kepulauan Riau, Indonesia'),
    (5, 'Puri Industrial Park, Baloi Permai, Batam Kota, Kepulauan Riau, Indonesia'),
    (6, 'Batam Industrial Park, Jalan Beringin Lot 312-313, Muka Kuning Batam, Kepulauan Riau, Indonesia')
) AS v(code, address) ON v.code = c.customer_code
WHERE NOT EXISTS (
    SELECT 1 FROM {{schema}}.customer_addresses a
    WHERE a.customer_id = c.id AND a.is_primary = TRUE
);
