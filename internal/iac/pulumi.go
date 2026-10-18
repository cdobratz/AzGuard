package iac

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"

	"github.com/azguard/azguard/internal/pricing"
)

type pulumiStep struct {
	Op       string         `json:"op"`
	URN      string         `json:"urn"`
	Type     string         `json:"type"`
	NewState map[string]any `json:"newState"`
	Inputs   map[string]any `json:"inputs"`
}

func estimatePulumiDoc(body []byte, book *pricing.Book) (Estimate, error) {
	var doc struct {
		Steps []pulumiStep `json:"steps"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return Estimate{}, err
	}
	return finish("pulumi", pulumiLines(doc.Steps, book)), nil
}

func estimatePulumiNDJSON(body []byte, book *pricing.Book) (Estimate, bool, error) {
	sc := bufio.NewScanner(bytes.NewReader(body))
	var steps []pulumiStep
	saw := false
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var step pulumiStep
		if err := json.Unmarshal(line, &step); err != nil {
			return Estimate{}, false, nil
		}
		if step.URN == "" && step.Op == "" {
			continue
		}
		saw = true
		steps = append(steps, step)
	}
	if !saw {
		return Estimate{}, false, nil
	}
	return finish("pulumi", pulumiLines(steps, book)), true, nil
}

func pulumiLines(steps []pulumiStep, book *pricing.Book) []Line {
	var lines []Line
	for _, step := range steps {
		if step.Op != "create" && step.Op != "replace" {
			continue
		}
		typ := step.Type
		if typ == "" {
			typ = typeFromURN(step.URN)
		}
		attrs := step.Inputs
		if attrs == nil && step.NewState != nil {
			if in, ok := step.NewState["inputs"].(map[string]any); ok {
				attrs = in
			} else {
				attrs = step.NewState
			}
		}
		kind := kindPulumi(typ)
		address := step.URN
		if kind == "" {
			lines = append(lines, Line{Address: address, Kind: typ})
			continue
		}
		lines = append(lines, price(kind, address, attrs, book))
	}
	return lines
}

func typeFromURN(urn string) string {
	// urn:pulumi:stack::project::aws:ec2/natGateway:NatGateway::name
	parts := strings.Split(urn, "::")
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}
