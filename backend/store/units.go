package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// CompanyUnit is one entry of the company's unit-of-measure list. Value is
// what quotations store, sign and validate against -- the stable key; Label
// is what the customer document prints (the issue's m2 → m² case).
type CompanyUnit struct {
	Value string
	Label string
	// SortOrder is the unit's position in the list. The store assigns it from
	// the array position on write, and it never travels on the wire: the
	// ordered array itself is the order contract. api/units_mirrors_test.go
	// pins Value and Label on the wire and SortOrder's absence.
	SortOrder int
}

// unitColumnList is the single declaration of the company_uom columns the
// store reads and writes, shared between the SQL statements and the guard
// that proves every CompanyUnit field has a column: a column list written
// twice is a column list that drifts.
const unitColumnList = "value, label, sort_order"

// The shape bounds a unit value may take mirror plausibleUoM and uoMMaxLen in
// backend/quotation.go. The store cannot import the backend package (import
// cycle), so the rule is duplicated here with the guarantee that keeps the
// duplication honest: the two sides must change together, and each side pins
// itself with a test (backend/quotation.go's TestPlausibleUoM here; the store
// round trip and the API's update guard on this side).
const (
	uomMaxLen = 12 // a unit value is a short token, not a sentence
	// labelMaxLen caps what prints in the UoM column. The column renders at
	// 8% of the page width — a 52 CSS px track whose content box is ~31 CSS
	// px — and render/gpdf/layout_test.go's fit pin measures the seeded
	// labels against that box: twelve of the fourteen seeds fit and "Month"
	// and "Quarter" overflow, a pre-existing condition recorded there. The
	// cap is therefore the edit-time ceiling, not a promise that the widest
	// label fits: a label longer than roughly "Quarter" at 11 px overflows,
	// exactly as the pre-#337 catalog's "Month" already did.
	labelMaxLen = 20
)

// unitValueShape reports whether a value could be a unit label: the same
// alphabet plausibleUoM allows.
func unitValueShape(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '/', r == '.':
		default:
			return false
		}
	}
	return true
}

// checkCompanyUnits validates a full replacement list at the store boundary:
// never empty, every value a shaped non-empty short token, every label a
// non-empty string, and no two values that differ only in case -- a
// case-insensitive duplicate would fold onto itself on the editor's dropdown,
// so it is refused rather than stored.
func checkCompanyUnits(units []CompanyUnit) error {
	if len(units) == 0 {
		return fmt.Errorf("at least one unit of measure is required")
	}
	seen := make(map[string]bool, len(units))
	for i, unit := range units {
		value := strings.TrimSpace(unit.Value)
		if value == "" {
			return fmt.Errorf("unit %d: value is required", i+1)
		}
		if len(value) > uomMaxLen {
			return fmt.Errorf("unit %d: value %q is too long", i+1, value)
		}
		if !unitValueShape(value) {
			return fmt.Errorf("unit %d: value %q is not shaped like a unit label", i+1, unit.Value)
		}
		label := strings.TrimSpace(unit.Label)
		if label == "" {
			return fmt.Errorf("unit %d: label is required", i+1)
		}
		if len(label) > labelMaxLen {
			return fmt.Errorf("unit %d: label %q is too long", i+1, label)
		}
		folded := strings.ToLower(value)
		if seen[folded] {
			return fmt.Errorf("unit %d: value %q duplicates an earlier unit (case-insensitively)", i+1, unit.Value)
		}
		seen[folded] = true
	}
	return nil
}

// unitListContains reports whether value is on the list, matching exactly
// (case-sensitive): company_profile.canonical_uom references
// company_uom.value byte for byte (migration 026), so a value that matches
// only case-insensitively is absent as far as the database is concerned.
func unitListContains(units []CompanyUnit, value string) bool {
	for _, unit := range units {
		if unit.Value == value {
			return true
		}
	}
	return false
}

// GetCompanyUnits reads the company's unit list in display order. Every
// signed-in role may read it -- the units print on customer documents, so
// hiding them from staff would break their previews (the same reasoning the
// logo endpoints carry).
func (s *Store) GetCompanyUnits(ctx context.Context) ([]CompanyUnit, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return nil, err
	}
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM %s.company_uom ORDER BY sort_order, id`,
		unitColumnList, s.configSchema()))
	if err != nil {
		return nil, fmt.Errorf("get company units: %w", err)
	}
	defer rows.Close()
	units := []CompanyUnit{}
	for rows.Next() {
		var unit CompanyUnit
		if err := rows.Scan(&unit.Value, &unit.Label, &unit.SortOrder); err != nil {
			return nil, fmt.Errorf("scan company unit: %w", err)
		}
		units = append(units, unit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get company units: %w", err)
	}
	return units, nil
}

// UpdateCompanyUnits full-replaces the company's unit list with the given
// array, in the given order. The canonical unit must survive the replacement
// (the profile's canonical_uom FK would refuse the transaction otherwise, and
// the store refuses earlier with a message naming the unit). The replacement
// runs in one transaction so the list is never observable half-replaced, and
// the FK -- DEFERRABLE, see migration 026 -- is checked at commit, when the
// canonical row is back in place.
func (s *Store) UpdateCompanyUnits(ctx context.Context, units []CompanyUnit) ([]CompanyUnit, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return nil, err
	}
	if err := checkCompanyUnits(units); err != nil {
		return nil, err
	}
	profile, err := s.GetCompanyProfile(ctx)
	if err != nil {
		return nil, err
	}
	if !unitListContains(units, profile.CanonicalUoM) {
		return nil, fmt.Errorf("the canonical unit %q must stay on the list", profile.CanonicalUoM)
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin company units update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.company_uom`, s.configSchema())); err != nil {
		return nil, fmt.Errorf("replace company units: %w", err)
	}
	for i, unit := range units {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.company_uom (%s) VALUES ($1, $2, $3)`,
			s.configSchema(), unitColumnList),
			strings.TrimSpace(unit.Value), strings.TrimSpace(unit.Label), i+1); err != nil {
			return nil, fmt.Errorf("replace company units: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("replace company units: %w", err)
	}
	return s.GetCompanyUnits(ctx)
}
