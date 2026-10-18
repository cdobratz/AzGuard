package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memStore struct {
	m      map[string]string
	setErr error
}

func (s *memStore) Get(service, user string) (string, error) {
	v, ok := s.m[service+"\n"+user]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (s *memStore) Set(service, user, secret string) error {
	if s.setErr != nil {
		return s.setErr
	}
	if s.m == nil {
		s.m = map[string]string{}
	}
	s.m[service+"\n"+user] = secret
	return nil
}

func TestMigrateSecretsOffDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := "azure:\n  subscription_id: sub\n  client_secret: super-secret\naws:\n  access_key: AKIAEXAMPLE\n  secret_key: s3cr3t-value\n  session_token: sess-token\n  region: us-east-1\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &memStore{}
	cfg := &Config{}
	if err := applySecrets(path, cfg, store); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, secret := range []string{"super-secret", "AKIAEXAMPLE", "s3cr3t-value", "sess-token"} {
		if strings.Contains(text, secret) {
			t.Fatalf("config still contains %q:\n%s", secret, text)
		}
	}
	if !strings.Contains(text, "us-east-1") || !strings.Contains(text, "sub") {
		t.Fatalf("non-secrets dropped:\n%s", text)
	}
	if cfg.Azure.ClientSecret != "super-secret" || cfg.AWS.AccessKey != "AKIAEXAMPLE" || cfg.AWS.SecretKey != "s3cr3t-value" || cfg.AWS.SessionToken != "sess-token" {
		t.Fatalf("memory = %+v %+v", cfg.Azure, cfg.AWS)
	}

	again := &Config{}
	if err := applySecrets(path, again, store); err != nil {
		t.Fatal(err)
	}
	if again.AWS.SecretKey != "s3cr3t-value" {
		t.Fatalf("keychain reload = %+v", again.AWS)
	}
}

func TestMigrateLeavesFileWhenKeychainFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := "azure:\n  client_secret: keep-me\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &memStore{setErr: errors.New("no keychain")}
	cfg := &Config{Azure: AzureConfig{ClientSecret: "keep-me"}}
	if err := applySecrets(path, cfg, store); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "keep-me") {
		t.Fatalf("file was rewritten without a keychain:\n%s", body)
	}
}

func TestEnvOverridesKeychain(t *testing.T) {
	t.Setenv("AZURE_CLIENT_SECRET", "from-env")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "env-secret")
	store := &memStore{m: map[string]string{
		keyringService + "\nazure.client_secret": "from-keychain",
		keyringService + "\naws.secret_key":      "from-keychain",
	}}
	cfg := &Config{}
	if err := applySecrets("", cfg, store); err != nil {
		t.Fatal(err)
	}
	if cfg.Azure.ClientSecret != "from-env" || cfg.AWS.SecretKey != "env-secret" {
		t.Fatalf("got azure %q aws %q", cfg.Azure.ClientSecret, cfg.AWS.SecretKey)
	}
}
