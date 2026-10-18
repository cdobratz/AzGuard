package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAccountKeepsSecretsOut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := "azure:\n  subscription_id: sub-1\n  client_secret: super-secret\nstorage:\n  path: data.db\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	err := WriteAccount(path, AccountConfig{
		Azure:         "payg",
		AWS:           "paid",
		AWSSpendLimit: "no",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "payg") || !strings.Contains(text, "paid") || !strings.Contains(text, "sub-1") {
		t.Fatalf("account section missing:\n%s", text)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}
