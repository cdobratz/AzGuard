package iac

import (
	"encoding/json"

	"github.com/cdobratz/azguard/internal/pricing"
)

func estimateCloudFormation(body []byte, book *pricing.Book) (Estimate, error) {
	var doc struct {
		Resources map[string]struct {
			Type       string         `json:"Type"`
			Properties map[string]any `json:"Properties"`
		} `json:"Resources"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return Estimate{}, err
	}
	var lines []Line
	for name, res := range doc.Resources {
		kind := kindCloudFormation(res.Type)
		if kind == "" {
			continue
		}
		lines = append(lines, price(kind, name, res.Properties, book))
	}
	return finish("cloudformation", lines), nil
}
