package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type TokenProvider func() (string, error)

var TokenProviders = map[string]TokenProvider{
	"cli":               GetCLIToken,
	"service_principal": nil,
	"managed_identity":  GetMIToken,
}

func GetCLIToken() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "az", "account", "get-access-token", "--resource", "https://management.azure.com", "--output", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Azure CLI token: %w", err)
	}

	var result struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}

	return result.AccessToken, nil
}

func GetSPToken(tenantID, clientID, clientSecret string) (string, error) {
	url := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)

	data := fmt.Sprintf(
		"grant_type=client_credentials&client_id=%s&client_secret=%s&scope=https://management.azure.com/.default",
		clientID, clientSecret,
	)

	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed with status: %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.AccessToken, nil
}

func GetMIToken() (string, error) {
	endpoint := os.Getenv("MSI_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://169.254.169.254/metadata/identity/oauth2/token"
	}

	clientID := os.Getenv("MSI_CLIENT_ID")

	url := fmt.Sprintf("%s?resource=https://management.azure.com&api-version=2018-02-01", endpoint)
	if clientID != "" {
		url += "&client_id=" + clientID
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata", "true")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("managed identity token request failed with status: %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.AccessToken, nil
}

func NewTokenProvider(authMethod string, config map[string]string) (TokenProvider, error) {
	switch authMethod {
	case "cli":
		return GetCLIToken, nil
	case "service_principal":
		return func() (string, error) {
			return GetSPToken(
				config["tenant_id"],
				config["client_id"],
				config["client_secret"],
			)
		}, nil
	case "managed_identity":
		return GetMIToken, nil
	default:
		return nil, fmt.Errorf("unknown auth method: %s", authMethod)
	}
}

// GetSubscriptionIDFromCLI retrieves the default subscription ID from Azure CLI
func GetSubscriptionIDFromCLI() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "az", "account", "show", "--output", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Azure account info: %w (run 'az login' first)", err)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("failed to parse account info: %w", err)
	}

	if result.ID == "" {
		return "", fmt.Errorf("no subscription ID found in Azure CLI output")
	}

	return result.ID, nil
}

// ValidateSubscriptionID checks if a subscription ID is valid
func ValidateSubscriptionID(subscriptionID string) error {
	if subscriptionID == "" {
		return fmt.Errorf("subscription ID is empty")
	}

	// Check for common placeholder values
	placeholders := []string{"providers", "YOUR_SUBSCRIPTION_ID", "YOUR_SUB_ID", "<subscription-id>", "subscription-id"}
	for _, placeholder := range placeholders {
		if strings.EqualFold(subscriptionID, placeholder) {
			return fmt.Errorf("subscription ID appears to be a placeholder value ('%s'). Run 'azguard config set subscription YOUR_ACTUAL_SUBSCRIPTION_ID' or 'az login' to configure", subscriptionID)
		}
	}

	// Validate GUID format (Azure subscription IDs are GUIDs)
	// Format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	guidPattern := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	if !guidPattern.MatchString(subscriptionID) {
		return fmt.Errorf("subscription ID '%s' does not match the expected GUID format. Run 'az account list' to find your subscription ID", subscriptionID)
	}

	return nil
}
