// Package events is the normalized shape of a cloud control-plane record.
package events

import "time"

// Principal is who performed an operation, as far as the provider log shows.
type Principal struct {
	Display     string
	Type        string
	ARN         string
	AccessKeyID string
	Caller      string
	IDTyp       string
	AppID       string
	UPN         string
}

// Event is one Azure Activity Log record or one CloudTrail event.
type Event struct {
	ID         string
	Provider   string
	Name       string
	ResourceID string
	Region     string
	At         time.Time
	Status     string
	Principal  Principal
	// Detail carries optional request fields (instance type, volume size, endpoint type).
	Detail map[string]string
}
