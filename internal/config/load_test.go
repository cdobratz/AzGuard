package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadExpandsPathsAndReadsAccount(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", t.TempDir())

	dir := filepath.Join(home, ".azguard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("azure:\n  auth_method: cli\n  subscription_id: 11111111-1111-1111-1111-111111111111\nstorage:\n  path: ~/data.db\naccount:\n  azure: free\n  azure_credit_remaining: \"40\"\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Azure.SubscriptionID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("subscription %s", cfg.Azure.SubscriptionID)
	}
	if cfg.Storage.Path != filepath.Join(home, "data.db") && cfg.Storage.Path != home+"/data.db" {
		t.Fatalf("storage path %s", cfg.Storage.Path)
	}
	if cfg.Account.Azure != "free" || cfg.Account.AzureCreditRemaining != "40" {
		t.Fatalf("account %+v", cfg.Account)
	}
	if Get().Azure.AuthMethod != "cli" {
		t.Fatalf("get %+v", Get())
	}
	if err := Set("azure.auth_method", "cli"); err != nil {
		t.Fatal(err)
	}
	if GetString("azure.subscription_id") == "" {
		t.Fatal("expected subscription from viper")
	}
}

func TestLoadFallsBackToAzureCLI(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	home := t.TempDir()
	bin := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", bin)
	script := filepath.Join(bin, "az")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '{\"id\":\"33333333-3333-3333-3333-333333333333\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".azguard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("azure:\n  subscription_id: YOUR_SUB_ID\nstorage:\n  path: $HOME/costs.db\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Azure.SubscriptionID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("subscription %s", cfg.Azure.SubscriptionID)
	}
	if cfg.Storage.Path != home+"/costs.db" {
		t.Fatalf("path %s", cfg.Storage.Path)
	}
	if cfg.Azure.AuthMethod != "cli" {
		t.Fatalf("auth %s", cfg.Azure.AuthMethod)
	}
}

func TestLoadMissingAndInvalidFile(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", t.TempDir())
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Azure.AuthMethod != "cli" || cfg.Storage.Path == "" {
		t.Fatalf("defaults %+v", cfg)
	}

	viper.Reset()
	dir := filepath.Join(home, ".azguard")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("azure: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(""); err == nil {
		t.Fatal("expected invalid yaml")
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if expandHome("~/x") != home+"/x" {
		t.Fatalf("tilde %s", expandHome("~/x"))
	}
	if expandHome("$HOME/y") != home+"/y" {
		t.Fatalf("dollar %s", expandHome("$HOME/y"))
	}
	if expandHome("/tmp/z") != "/tmp/z" {
		t.Fatalf("abs %s", expandHome("/tmp/z"))
	}
}
