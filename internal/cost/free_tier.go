package cost

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cdobratz/azguard/configs"
	"gopkg.in/yaml.v3"
)

type FreeTierConfig struct {
	Version  int                     `yaml:"version"`
	Services map[string]ServiceLimit `yaml:"services"`
	Budgets  map[string]BudgetPreset `yaml:"budgets"`
}

type ServiceLimit struct {
	Description      string  `yaml:"description"`
	Limit            float64 `yaml:"limit"`
	Unit             string  `yaml:"unit"`
	Duration         string  `yaml:"duration"`
	WarningThreshold float64 `yaml:"warning_threshold"`
}

type BudgetPreset struct {
	Amount      float64 `yaml:"amount"`
	Description string  `yaml:"description"`
}

// LoadFreeTierConfig reads the embedded Azure free-tier table.
// A file at ~/.azguard/free_tier_limits.yaml replaces the embedded copy.
func LoadFreeTierConfig() (*FreeTierConfig, error) {
	return loadLimitTable("free_tier_limits.yaml", configs.AzureFreeTierYAML)
}

// LoadAWSFreeTierConfig reads the embedded AWS free-tier table.
// A file at ~/.azguard/aws_free_tier_limits.yaml replaces the embedded copy.
func LoadAWSFreeTierConfig() (*FreeTierConfig, error) {
	return loadLimitTable("aws_free_tier_limits.yaml", configs.AWSFreeTierYAML)
}

func loadLimitTable(name string, embedded []byte) (*FreeTierConfig, error) {
	data, err := readLimitOverride(name)
	if err != nil {
		return nil, err
	}
	source := "embedded " + name
	if data == nil {
		data = embedded
	} else {
		source = name
	}
	return parseLimitTable(source, data)
}

func readLimitOverride(name string) ([]byte, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, ".azguard", name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

func parseLimitTable(source string, data []byte) (*FreeTierConfig, error) {
	cfg := &FreeTierConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", source, err)
	}
	if cfg.Version < 1 {
		return nil, fmt.Errorf("%s: version is required", source)
	}
	return cfg, nil
}

type ResourceStatus string

const (
	StatusFree    ResourceStatus = "free"
	StatusWarning ResourceStatus = "warning"
	StatusOverage ResourceStatus = "overage"
	StatusUnknown ResourceStatus = "unknown"
)

type ServiceUsage struct {
	ServiceName string
	Used        float64
	Limit       float64
	Unit        string
	Status      ResourceStatus
	PercentUsed float64
}

func CheckServiceUsage(usage float64, limit *ServiceLimit) ServiceUsage {
	if limit == nil || limit.Limit <= 0 {
		return ServiceUsage{Status: StatusUnknown, Used: usage}
	}
	if usage < 0 {
		usage = 0
	}

	percentUsed := usage / limit.Limit
	var status ResourceStatus

	if percentUsed >= 1.0 {
		status = StatusOverage
	} else if limit.WarningThreshold > 0 && percentUsed >= limit.WarningThreshold {
		status = StatusWarning
	} else {
		status = StatusFree
	}

	return ServiceUsage{
		ServiceName: "",
		Used:        usage,
		Limit:       limit.Limit,
		Unit:        limit.Unit,
		Status:      status,
		PercentUsed: percentUsed * 100,
	}
}
