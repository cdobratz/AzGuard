package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"
)

const keyringService = "azguard"

// secretKeys are config fields that must not stay in the YAML file.
// Env is the environment variable that overrides the keychain.
var secretKeys = []secretKey{
	{section: "azure", field: "client_secret", env: "AZURE_CLIENT_SECRET", apply: func(c *Config, v string) { c.Azure.ClientSecret = v }, get: func(c *Config) string { return c.Azure.ClientSecret }},
	{section: "aws", field: "access_key", env: "AWS_ACCESS_KEY_ID", apply: func(c *Config, v string) { c.AWS.AccessKey = v }, get: func(c *Config) string { return c.AWS.AccessKey }},
	{section: "aws", field: "secret_key", env: "AWS_SECRET_ACCESS_KEY", apply: func(c *Config, v string) { c.AWS.SecretKey = v }, get: func(c *Config) string { return c.AWS.SecretKey }},
	{section: "aws", field: "session_token", env: "AWS_SESSION_TOKEN", apply: func(c *Config, v string) { c.AWS.SessionToken = v }, get: func(c *Config) string { return c.AWS.SessionToken }},
}

type secretKey struct {
	section string
	field   string
	env     string
	apply   func(*Config, string)
	get     func(*Config) string
}

func (k secretKey) account() string { return k.section + "." + k.field }

type secretStore interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
}

type keyringStore struct{}

func (keyringStore) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (keyringStore) Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

// defaultStore is the OS keychain. Tests replace it.
var defaultStore secretStore = keyringStore{}

// applySecrets moves plaintext secrets out of the config file when the keychain
// accepts them, fills empty fields from the keychain, then lets env vars win.
func applySecrets(path string, cfg *Config, store secretStore) error {
	if store == nil {
		store = defaultStore
	}
	if path != "" {
		if err := migrateFile(path, store); err != nil {
			return err
		}
	}
	for _, key := range secretKeys {
		if key.get(cfg) != "" {
			continue
		}
		v, err := store.Get(keyringService, key.account())
		if err != nil || v == "" {
			continue
		}
		key.apply(cfg, v)
	}
	for _, key := range secretKeys {
		if v := os.Getenv(key.env); v != "" {
			key.apply(cfg, v)
		}
	}
	return nil
}

func migrateFile(path string, store secretStore) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("read config secrets: %w", err)
	}
	type item struct {
		key   secretKey
		value string
	}
	var found []item
	for _, key := range secretKeys {
		section, _ := doc[key.section].(map[string]any)
		if section == nil {
			continue
		}
		v, _ := section[key.field].(string)
		if strings.TrimSpace(v) == "" {
			continue
		}
		found = append(found, item{key: key, value: v})
	}
	if len(found) == 0 {
		return nil
	}
	for _, item := range found {
		if err := store.Set(keyringService, item.key.account(), item.value); err != nil {
			// Leave the file unchanged so the secret is not dropped when the keychain is unavailable.
			return nil
		}
	}
	for _, item := range found {
		section := doc[item.key.section].(map[string]any)
		delete(section, item.key.field)
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}
