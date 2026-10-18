package cost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLimitEntriesHaveSourceAndDate(t *testing.T) {
	root := moduleRoot(t)
	files := []string{
		filepath.Join(root, "configs", "free_tier_limits.yaml"),
		filepath.Join(root, "configs", "aws_free_tier_limits.yaml"),
	}
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Services map[string]struct {
				SourceURL  string `yaml:"source_url"`
				VerifiedOn string `yaml:"verified_on"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal(body, &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Services) == 0 {
			t.Fatalf("%s has no services", path)
		}
		for name, svc := range doc.Services {
			if strings.TrimSpace(svc.SourceURL) == "" || strings.TrimSpace(svc.VerifiedOn) == "" {
				t.Fatalf("%s service %s is missing source_url or verified_on", path, name)
			}
		}
	}
	aws, err := os.ReadFile(filepath.Join(root, "configs", "aws_free_tier_limits.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(aws)
	if !strings.Contains(text, "https://aws.amazon.com/free/") || !strings.Contains(text, "Free plan") || !strings.Contains(text, "Paid plan") {
		t.Fatal("aws limits file should cite https://aws.amazon.com/free/ and name Free plan vs Paid plan")
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
