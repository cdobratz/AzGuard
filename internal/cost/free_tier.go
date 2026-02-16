package cost

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type FreeTierConfig struct {
	Services map[string]ServiceLimit `yaml:"services"`
	Budgets map[string]BudgetPreset `yaml:"budgets"`
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

func LoadFreeTierConfig() (*FreeTierConfig, error) {
	paths := []string{
		"configs/free_tier_limits.yaml",
		"./configs/free_tier_limits.yaml",
		filepath.Join(os.Getenv("HOME"), ".azguard", "free_tier_limits.yaml"),
	}

	var config *FreeTierConfig

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			config = &FreeTierConfig{}
			if err := yaml.Unmarshal(data, config); err == nil {
				return config, nil
			}
		}
	}

	// Return default config if no file found
	return &FreeTierConfig{
		Services: map[string]ServiceLimit{
			"virtual_machines": {
				Description:      "B1s VM hours",
				Limit:            750,
				Unit:             "hours",
				Duration:         "12 months",
				WarningThreshold: 0.8,
			},
			"blob_storage": {
				Description:      "Hot Blob Storage",
				Limit:            5,
				Unit:             "GB",
				Duration:         "always free",
				WarningThreshold: 0.8,
			},
			"functions": {
				Description:      "Azure Functions",
				Limit:            1000000,
				Unit:             "executions",
				Duration:         "always free",
				WarningThreshold: 0.8,
			},
		},
		Budgets: map[string]BudgetPreset{
			"tiny":     {Amount: 1, Description: "Strict budget"},
			"small":    {Amount: 5, Description: "Small budget"},
			"medium":   {Amount: 10, Description: "Medium budget"},
			"moderate": {Amount: 20, Description: "Higher budget"},
		},
	}, nil
}

type ResourceStatus string

const (
	StatusFree     ResourceStatus = "free"
	StatusWarning  ResourceStatus = "warning"
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
	if limit == nil {
		return ServiceUsage{Status: StatusUnknown}
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
