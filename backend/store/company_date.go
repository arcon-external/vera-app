package store

import "time"

// companyLocation is the company's commercial date context: WIB
// (Asia/Jakarta, UTC+7). There is no company or user timezone setting yet, so
// this is a fixed zone rather than time.Local: the API server may run anywhere
// (Cloud Run runs UTC) while the office that dates a received Purchase Order
// works in Jakarta. When a real setting arrives, it replaces this one value.
var companyLocation = time.FixedZone("WIB", 7*60*60)

// companyToday returns "today" in the company's date context, represented as
// midnight UTC because that is how a Postgres DATE is stored and compared. It
// converts the instant first: 2026-10-20T18:30:00Z is already 2026-10-21 in
// WIB, so a default received date must not follow the server's clock.
func companyToday(now time.Time) time.Time {
	local := now.In(companyLocation)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}
