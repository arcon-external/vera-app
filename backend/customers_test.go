package backend

import "testing"

// newTestQuotation builds a Quotation with the legacy client directory
// seeded, mirroring the mock-era rows the dev database carries. Production
// code never hard-codes clients; this fixture exists only so unit tests
// can exercise Prepare without a store or a server.
func newTestQuotation() *Quotation {
	q := &Quotation{}
	q.customerDirectory = testCustomerDirectory()
	return q
}

func testCustomerDirectory() []ManagedClient {
	return []ManagedClient{
		{ID: 1, Code: 2, Name: "PT. Infineon Technologies Batam", PrimaryAddress: "Batamindo Industrial Park, Lot 317, Mukakuning, Kota Batam, Kepulauan Riau, Indonesia"},
		{ID: 2, Code: 3, Name: "PT. H-Tech Oilfield Equipment, NOV XL Systems", PrimaryAddress: "Kecamatan Nongsa, Kota Batam, Kepulauan Riau, Indonesia"},
		{ID: 3, Code: 4, Name: "PT. Datalec Global Solution", PrimaryAddress: "Ruko Pollux Habibie No.E16, Kota Batam, Kepulauan Riau, Indonesia"},
		{ID: 4, Code: 5, Name: "PT. Panasonic Industrial Devices Batam", PrimaryAddress: "Puri Industrial Park, Baloi Permai, Batam Kota, Kepulauan Riau, Indonesia"},
		{ID: 5, Code: 6, Name: "PT. Surya Teknologi Batam", PrimaryAddress: "Batam Industrial Park, Jalan Beringin Lot 312-313, Muka Kuning Batam, Kepulauan Riau, Indonesia"},
	}
}

func TestResolveCustomerFromDirectory(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	q := newTestQuotation()
	company, address, err := q.resolveCustomer(4)
	if err != nil {
		t.Fatalf("resolve 4: %v", err)
	}
	if company != "PT. Datalec Global Solution" {
		t.Errorf("company = %q; want Datalec", company)
	}
	if address == "" {
		t.Error("address is empty")
	}
	if _, _, err := q.resolveCustomer(7); err == nil {
		t.Error("unknown code resolved from a seeded directory; want invalid customer ID")
	}
}

func TestResolveCustomerRefreshesOnMiss(t *testing.T) {
	t.Setenv("VERA_NO_KEYRING", "1")
	q := &Quotation{}
	// Empty directory with no server behind api(): the miss must fail
	// loudly, never invent a client.
	if _, _, err := q.resolveCustomer(2); err == nil {
		t.Error("empty directory resolved; want invalid customer ID")
	}
}
