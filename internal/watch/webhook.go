package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Payload is a Slack- and Discord-compatible webhook body.
// Slack displays `text`. Discord displays `content`. Both are set to the same message.
// `attachments` is the Slack attachment shape; Discord ignores unknown fields.
type Payload struct {
	Text        string       `json:"text"`
	Content     string       `json:"content"`
	Username    string       `json:"username"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Attachment is one Slack attachment. Discord shows `content` and ignores this.
type Attachment struct {
	Color  string  `json:"color,omitempty"`
	Title  string  `json:"title"`
	Text   string  `json:"text"`
	Fields []Field `json:"fields,omitempty"`
}

// Field is one attachment field.
type Field struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// WebhookSchema documents the POST body `azguard watch --webhook` sends.
const WebhookSchema = `{
  "text": "Slack message text. Same words as content.",
  "content": "Discord message text. Same words as text.",
  "username": "azguard",
  "attachments": [
    {
      "color": "#e67e22 when an automation principal is included, otherwise #3498db",
      "title": "New billable resource",
      "text": "One block per resource",
      "fields": [
        {"title": "provider", "value": "azure or aws", "short": true},
        {"title": "operation", "value": "control-plane operation", "short": true},
        {"title": "resource", "value": "resource id", "short": false},
        {"title": "principal", "value": "who created it", "short": true},
        {"title": "automation", "value": "yes: reason, or no", "short": true},
        {"title": "estimated_monthly_usd", "value": "list-price estimate", "short": true}
      ]
    }
  ]
}
`

// BuildPayload turns alerts into one webhook body.
func BuildPayload(alerts []Alert) Payload {
	var lines []string
	auto := false
	var attachments []Attachment
	for _, a := range alerts {
		if a.Automation {
			auto = true
		}
		line := FormatLine(a)
		lines = append(lines, line)
		flag := "no"
		if a.Automation {
			flag = "yes: " + a.AutomationReason
		}
		attachments = append(attachments, Attachment{
			Title: "New billable resource",
			Text:  line,
			Fields: []Field{
				{Title: "provider", Value: a.Provider, Short: true},
				{Title: "operation", Value: a.Operation, Short: true},
				{Title: "resource", Value: empty(a.ResourceID, "-"), Short: false},
				{Title: "principal", Value: empty(a.Principal, "unknown"), Short: true},
				{Title: "automation", Value: flag, Short: true},
				{Title: "estimated_monthly_usd", Value: fmt.Sprintf("%.2f", a.EstimatedMonthlyUSD), Short: true},
			},
		})
	}
	color := "#3498db"
	if auto {
		color = "#e67e22"
	}
	for i := range attachments {
		attachments[i].Color = color
	}
	text := "azguard: billable resource created\n" + strings.Join(lines, "\n")
	return Payload{
		Text:        text,
		Content:     text,
		Username:    "azguard",
		Attachments: attachments,
	}
}

// Post sends payload to a Slack or Discord incoming webhook.
func Post(ctx context.Context, endpoint string, payload Payload, client *http.Client) error {
	if endpoint == "" {
		return fmt.Errorf("webhook url is empty")
	}
	if client == nil {
		client = http.DefaultClient
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "azguard")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(respBody))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("webhook status %d: %s", resp.StatusCode, msg)
	}
	return nil
}
