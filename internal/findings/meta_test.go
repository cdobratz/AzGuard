package findings

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFindingMetaOmittedFromJSON(t *testing.T) {
	f := Finding{
		ResourceID: "vol-1",
		Provider:   "aws",
		Tags:       map[string]string{"azguard:keep": "true"},
		CreatedAt:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		PriceKey:   "hidden",
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, leak := range []string{"azguard", "created_at", "hidden", "price_key"} {
		if strings.Contains(body, leak) {
			t.Fatalf("resources JSON leaked %q: %s", leak, body)
		}
	}
}
