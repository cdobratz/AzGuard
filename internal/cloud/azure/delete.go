package azure

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// azureDeleteAPI is the ARM api-version used to delete one resource type.
var azureDeleteAPI = map[string]string{
	"microsoft.compute/disks":                  "2024-03-02",
	"microsoft.compute/snapshots":              "2024-03-02",
	"microsoft.compute/virtualmachines":        "2024-03-01",
	"microsoft.network/publicipaddresses":      "2024-01-01",
	"microsoft.network/networkinterfaces":      "2024-01-01",
	"microsoft.network/natgateways":            "2024-01-01",
	"microsoft.web/serverfarms":                "2023-12-01",
	"microsoft.operationalinsights/workspaces": "2023-09-01",
}

// DeleteResource deletes one ARM resource in the reader's subscription.
// HTTP 200, 202, and 204 are success. 202 means Azure accepted the delete and
// finishes it in the background. 404 is success so a rerun can finish.
func (r *Reader) DeleteResource(ctx context.Context, resourceID string) error {
	if r == nil {
		return fmt.Errorf("azure reader is not configured")
	}
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" || strings.ContainsAny(resourceID, " \t\r\n?#") || strings.Contains(resourceID, "..") {
		return fmt.Errorf("refusing to delete %q: unexpected resource id", resourceID)
	}
	if err := ValidateSubscriptionID(r.SubscriptionID); err != nil {
		return err
	}
	prefix := "/subscriptions/" + strings.ToLower(r.SubscriptionID) + "/"
	if !strings.HasPrefix(strings.ToLower(resourceID), prefix) {
		return fmt.Errorf("refusing to delete %q: it is outside the configured subscription", resourceID)
	}
	kind := armType(resourceID)
	ver, ok := azureDeleteAPI[kind]
	if !ok {
		return fmt.Errorf("refusing to delete %q: unsupported resource type %q", resourceID, kind)
	}
	q := url.Values{}
	q.Set("api-version", ver)
	endpoint := r.base() + resourceID + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	if r.Token == nil {
		return fmt.Errorf("azure token provider is not configured")
	}
	token, err := r.Token()
	if err != nil {
		return fmt.Errorf("azure token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "azguard")
	client := r.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusAccepted, http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		msg := strings.TrimSpace(string(body))
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return fmt.Errorf("azure delete %s status %d: %s", resourceID, resp.StatusCode, msg)
	}
}

func armType(id string) string {
	low := strings.ToLower(id)
	const marker = "/providers/"
	i := strings.Index(low, marker)
	if i < 0 {
		return ""
	}
	rest := low[i+len(marker):]
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}
