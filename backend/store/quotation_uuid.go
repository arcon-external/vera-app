package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewQuotationUUID generates the external identity for a new quotation.
// Generating it in Go rather than relying on the column default lets the
// export path know the verification handle before it renders the document,
// so the printed QR and the stored snapshot always agree.
func NewQuotationUUID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate quotation uuid: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40 // version 4
	raw[8] = (raw[8] & 0x3f) | 0x80 // RFC 4122 variant
	encoded := hex.EncodeToString(raw)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32], nil
}

// GetQuotationUUIDByNumber resolves an existing quotation's external
// identity, so a revision's verification handle matches its quotation.
func (s *Store) GetQuotationUUIDByNumber(ctx context.Context, quoteNumber string) (string, error) {
	pool, err := s.poolOrError()
	if err != nil {
		return "", err
	}
	var uuid string
	if err := pool.QueryRow(ctx, fmt.Sprintf(
		`SELECT uuid::text FROM %s.quotations WHERE quote_number = $1`, s.configSchema()),
		quoteNumber).Scan(&uuid); err != nil {
		return "", fmt.Errorf("quotation %q not found", quoteNumber)
	}
	return uuid, nil
}
