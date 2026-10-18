package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WriteAccount stores the init answers in the YAML config. It does not write cloud secrets.
func WriteAccount(path string, account AccountConfig) error {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path = filepath.Join(home, ".azguard", "config.yaml")
	}
	var doc map[string]any
	if body, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(body, &doc); err != nil {
			return fmt.Errorf("read config: %w", err)
		}
	}
	if doc == nil {
		doc = map[string]any{}
	}
	section, _ := doc["account"].(map[string]any)
	if section == nil {
		section = map[string]any{}
	}
	put := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		section[key] = value
	}
	put("azure", account.Azure)
	put("aws", account.AWS)
	put("aws_spend_limit", account.AWSSpendLimit)
	put("azure_credit_remaining", account.AzureCreditRemaining)
	put("azure_credit_expires", account.AzureCreditExpires)
	put("aws_credit_remaining", account.AWSCreditRemaining)
	put("aws_credit_expires", account.AWSCreditExpires)
	doc["account"] = section
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}
