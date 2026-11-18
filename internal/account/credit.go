package account

import (
	"strings"
	"time"
)

// APICredit is a read-only credit balance from a cloud API.
// Available is false when the API could not be read. Note is shown in that case.
type APICredit struct {
	Available bool
	Remaining float64
	Expires   string
	Source    string
	Note      string
}

// DaysLeft is the number of UTC calendar days from now until expires.
// A negative result means the date is in the past. The date result is YYYY-MM-DD.
func DaysLeft(expires string, now time.Time) (days int, date string, ok bool) {
	raw := strings.TrimSpace(expires)
	if len(raw) >= 10 {
		raw = raw[:10]
	}
	day, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return 0, "", false
	}
	nowDay := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	expDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	return int(expDay.Sub(nowDay).Hours() / 24), expDay.Format("2006-01-02"), true
}

// ApplyCredit merges a cloud API reading with the amount stored by azguard init.
// A Pay-As-You-Go or Paid profile is unchanged except for the credit lines.
// Callers must not turn the result into a percent of a fixed free-tier grant.
func ApplyCredit(base, stored Profile, api APICredit) Profile {
	out := base
	if out.CreditRemaining == nil {
		out.CreditRemaining = stored.CreditRemaining
	}
	if strings.TrimSpace(out.CreditExpires) == "" {
		out.CreditExpires = stored.CreditExpires
	}
	if !api.Available {
		if out.CreditRemaining != nil || strings.TrimSpace(out.CreditExpires) != "" {
			out.CreditSource = "config"
		}
		out.CreditNote = api.Note
		return out
	}
	remaining := api.Remaining
	out.CreditRemaining = &remaining
	if strings.TrimSpace(api.Expires) != "" {
		out.CreditExpires = strings.TrimSpace(api.Expires)
	}
	if api.Source != "" {
		out.CreditSource = api.Source
	}
	var notes []string
	if stored.CreditRemaining != nil && *stored.CreditRemaining != api.Remaining {
		notes = append(notes, "Config also stores a credit amount. The value from the cloud API is shown.")
	}
	if api.Note != "" {
		notes = append(notes, api.Note)
	}
	out.CreditNote = strings.Join(notes, " ")
	return out
}
