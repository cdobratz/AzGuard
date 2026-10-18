package iac

import (
	"encoding/json"

	"github.com/cdobratz/azguard/internal/pricing"
)

type tfPlan struct {
	ResourceChanges []tfChange `json:"resource_changes"`
}

type tfChange struct {
	Address string `json:"address"`
	Mode    string `json:"mode"`
	Type    string `json:"type"`
	Change  struct {
		Actions []string       `json:"actions"`
		After   map[string]any `json:"after"`
	} `json:"change"`
}

func estimateTerraform(body []byte, book *pricing.Book) (Estimate, error) {
	var plan tfPlan
	if err := json.Unmarshal(body, &plan); err != nil {
		return Estimate{}, err
	}
	var lines []Line
	for _, rc := range plan.ResourceChanges {
		if rc.Mode != "" && rc.Mode != "managed" {
			continue
		}
		if !containsCreate(rc.Change.Actions) {
			continue
		}
		kind := kindTerraform(rc.Type)
		if kind == "" {
			lines = append(lines, Line{Address: rc.Address, Kind: rc.Type})
			continue
		}
		lines = append(lines, price(kind, rc.Address, rc.Change.After, book))
	}
	return finish("terraform", lines), nil
}
