// Package iac estimates a monthly list price for infrastructure changes.
// Prices come from the bundled table. Terraform plans count creates only.
// A CloudFormation or CDK template is the stack's monthly run-rate, not a diff.
package iac

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cdobratz/azguard/internal/pricing"
	"gopkg.in/yaml.v3"
)

// Line is one priced resource.
type Line struct {
	Address    string
	Kind       string
	MonthlyUSD float64
	Priced     bool
}

// Estimate is the monthly run-rate of the resources a plan would create,
// or of every billable resource in a CloudFormation template.
type Estimate struct {
	Format     string
	Lines      []Line
	MonthlyUSD float64
	// Complete is true when every billable resource had a bundled price.
	Complete bool
}

// Parse reads a terraform show -json plan, a Pulumi preview JSON or NDJSON
// stream, or a CloudFormation/CDK template.
func Parse(body []byte, book *pricing.Book) (Estimate, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return Estimate{}, fmt.Errorf("iac: empty plan")
	}
	if book == nil {
		book = pricing.Bundled()
	}
	if body[0] != '{' && body[0] != '[' {
		if est, ok, err := estimateYAML(body, book); ok || err != nil {
			return est, err
		}
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err == nil {
		switch {
		case probe["resource_changes"] != nil:
			return estimateTerraform(body, book)
		case probe["steps"] != nil:
			return estimatePulumiDoc(body, book)
		case probe["Resources"] != nil:
			return estimateCloudFormation(body, book)
		case probe["artifacts"] != nil:
			return Estimate{}, fmt.Errorf("iac: CDK manifest needs a file path so templateFile can be loaded")
		}
	}
	if est, ok, err := estimatePulumiNDJSON(body, book); ok || err != nil {
		return est, err
	}
	return Estimate{}, fmt.Errorf("iac: unrecognized plan (want terraform show -json, pulumi preview JSON, or a CloudFormation template)")
}

// EstimateFile parses path. A CDK manifest loads templateFile entries next to it.
func EstimateFile(path string, book *pricing.Book) (Estimate, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Estimate{}, err
	}
	if book == nil {
		book = pricing.Bundled()
	}
	var probe map[string]json.RawMessage
	if json.Unmarshal(body, &probe) == nil && probe["artifacts"] != nil && probe["resource_changes"] == nil && probe["Resources"] == nil {
		return estimateCDKManifest(path, body, book)
	}
	return Parse(body, book)
}

func finish(format string, lines []Line) Estimate {
	est := Estimate{Format: format, Lines: lines, Complete: true}
	for _, line := range lines {
		if !line.Priced {
			est.Complete = false
			continue
		}
		est.MonthlyUSD += line.MonthlyUSD
	}
	est.MonthlyUSD = pricing.RoundUSD(est.MonthlyUSD)
	if len(lines) == 0 {
		est.Complete = true
	}
	return est
}

func estimateYAML(body []byte, book *pricing.Book) (Estimate, bool, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(body, &doc); err != nil || doc["Resources"] == nil {
		return Estimate{}, false, nil
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return Estimate{}, true, err
	}
	est, err := estimateCloudFormation(raw, book)
	return est, true, err
}

func estimateCDKManifest(path string, body []byte, book *pricing.Book) (Estimate, error) {
	var manifest struct {
		Artifacts map[string]struct {
			Type       string `json:"type"`
			Properties struct {
				TemplateFile string `json:"templateFile"`
			} `json:"properties"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return Estimate{}, fmt.Errorf("iac: cdk manifest: %w", err)
	}
	dir := filepath.Dir(path)
	var lines []Line
	found := false
	for _, art := range manifest.Artifacts {
		if art.Type != "aws:cloudformation:stack" || art.Properties.TemplateFile == "" {
			continue
		}
		found = true
		tpl := art.Properties.TemplateFile
		if !filepath.IsAbs(tpl) {
			tpl = filepath.Join(dir, tpl)
		}
		est, err := EstimateFile(tpl, book)
		if err != nil {
			return Estimate{}, err
		}
		lines = append(lines, est.Lines...)
	}
	if !found {
		return Estimate{}, fmt.Errorf("iac: cdk manifest has no CloudFormation templateFile")
	}
	return finish("cdk", lines), nil
}

func strAttr(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if t != "" {
				return t
			}
		case []any:
			if s := firstString(t); s != "" {
				return s
			}
			if len(t) > 0 {
				if obj, ok := t[0].(map[string]any); ok {
					if s := strAttr(obj, "name", "Name", "value"); s != "" {
						return s
					}
				}
			}
		case map[string]any:
			if s := strAttr(t, "name", "Name", "value"); s != "" {
				return s
			}
		}
	}
	return ""
}

func numAttr(m map[string]any, keys ...string) float64 {
	if m == nil {
		return 0
	}
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case float64:
			return t
		case string:
			var n float64
			if _, err := fmt.Sscan(t, &n); err == nil {
				return n
			}
		}
	}
	return 0
}

func listLen(m map[string]any, keys ...string) int {
	if m == nil {
		return 0
	}
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		if items, ok := v.([]any); ok {
			return len(items)
		}
	}
	return 0
}

func firstString(items []any) string {
	for _, item := range items {
		s, ok := item.(string)
		if ok && s != "" {
			return s
		}
	}
	return ""
}

func containsCreate(actions []string) bool {
	for _, a := range actions {
		if strings.EqualFold(a, "create") {
			return true
		}
	}
	return false
}
