package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cdobratz/azguard/internal/config"
)

func TestInitStoresAccountTypes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg = &config.Config{}

	cmd := initCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--azure", "payg", "--aws", "paid", "--aws-spend-limit", "no"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Azure payg") || !strings.Contains(buf.String(), "AWS paid") {
		t.Fatalf("output = %q", buf.String())
	}
	body, err := os.ReadFile(filepath.Join(home, ".azguard", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "payg") || !strings.Contains(text, "paid") {
		t.Fatalf("config = %s", text)
	}
	for _, secret := range []string{"client_secret", "access_key", "secret_key"} {
		if strings.Contains(text, secret) {
			t.Fatalf("init wrote %s:\n%s", secret, text)
		}
	}
}

func TestInitRejectsUnknownAccountType(t *testing.T) {
	cfg = &config.Config{}
	cmd := initCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--azure", "enterprise", "--aws", "paid"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for an unknown azure type")
	}
}
