package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cdobratz/azguard/internal/events"
)

type activityPage struct {
	Value    []activityRecord `json:"value"`
	NextLink string           `json:"nextLink"`
}

type activityRecord struct {
	EventDataID    string          `json:"eventDataId"`
	EventTimestamp string          `json:"eventTimestamp"`
	OperationName  json.RawMessage `json:"operationName"`
	Status         json.RawMessage `json:"status"`
	Caller         string          `json:"caller"`
	ResourceID     string          `json:"resourceId"`
	Claims         map[string]any  `json:"claims"`
	CorrelationID  string          `json:"correlationId"`
}

// ParseActivity reads one Activity Log page.
func ParseActivity(body []byte) ([]events.Event, string, error) {
	var page activityPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, "", fmt.Errorf("parse activity log: %w", err)
	}
	out := make([]events.Event, 0, len(page.Value))
	for _, rec := range page.Value {
		at, err := time.Parse(time.RFC3339Nano, rec.EventTimestamp)
		if err != nil {
			at, _ = time.Parse(time.RFC3339, rec.EventTimestamp)
		}
		id := rec.EventDataID
		if id == "" {
			id = rec.CorrelationID + "|" + localValue(rec.OperationName) + "|" + rec.ResourceID
		}
		p := principalFromClaims(rec.Caller, rec.Claims)
		out = append(out, events.Event{
			ID:         id,
			Provider:   "azure",
			Name:       localValue(rec.OperationName),
			ResourceID: rec.ResourceID,
			At:         at.UTC(),
			Status:     localValue(rec.Status),
			Principal:  p,
		})
	}
	return out, page.NextLink, nil
}

func localValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Value
	}
	return ""
}

func principalFromClaims(caller string, claims map[string]any) events.Principal {
	p := events.Principal{Caller: caller, Display: caller}
	for k, v := range claims {
		s, _ := v.(string)
		key := strings.ToLower(k)
		switch {
		case key == "idtyp" || strings.HasSuffix(key, "/idtyp"):
			p.IDTyp = s
		case key == "appid" || strings.HasSuffix(key, "/appid"):
			p.AppID = s
		case strings.HasSuffix(key, "/upn") || key == "upn":
			p.UPN = s
		case strings.HasSuffix(key, "/name") || key == "name":
			if s != "" {
				p.Display = s
			}
		}
	}
	if p.UPN != "" {
		p.Display = p.UPN
	}
	if p.IDTyp != "" {
		p.Type = p.IDTyp
	}
	return p
}

// ActivityEvents reads the subscription Activity Log between since and until.
func (r *Reader) ActivityEvents(ctx context.Context, since, until time.Time) ([]events.Event, error) {
	if err := ValidateSubscriptionID(r.SubscriptionID); err != nil {
		return nil, err
	}
	next := r.base() + r.ActivityPath(since, until)
	var all []events.Event
	for page := 0; page < 20 && next != ""; page++ {
		body, err := r.getAbsolute(ctx, next)
		if err != nil {
			return nil, err
		}
		ev, link, err := ParseActivity(body)
		if err != nil {
			return nil, err
		}
		all = append(all, ev...)
		next = link
	}
	return all, nil
}

func (r *Reader) getAbsolute(ctx context.Context, full string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	return r.do(req)
}
