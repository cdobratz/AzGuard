package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/cdobratz/azguard/internal/cloud/azure"
	"github.com/spf13/viper"
)

type Config struct {
	Azure   AzureConfig   `mapstructure:"azure"`
	AWS     AWSConfig     `mapstructure:"aws"`
	GCP     GCPConfig     `mapstructure:"gcp"`
	Storage StorageConfig `mapstructure:"storage"`
	Account AccountConfig `mapstructure:"account"`
}

type AzureConfig struct {
	AuthMethod     string `mapstructure:"auth_method"`
	SubscriptionID string `mapstructure:"subscription_id"`
	TenantID       string `mapstructure:"tenant_id"`
	ClientID       string `mapstructure:"client_id"`
	ClientSecret   string `mapstructure:"client_secret"`
}

type AWSConfig struct {
	AccessKey    string `mapstructure:"access_key"`
	SecretKey    string `mapstructure:"secret_key"`
	SessionToken string `mapstructure:"session_token"`
	Region       string `mapstructure:"region"`
}

type GCPConfig struct {
	ProjectID string `mapstructure:"project_id"`
}

type StorageConfig struct {
	Path string `mapstructure:"path"`
}

// AccountConfig is the answer from `azguard init`. These fields are not secrets.
type AccountConfig struct {
	Azure                string `mapstructure:"azure"`
	AWS                  string `mapstructure:"aws"`
	AWSSpendLimit        string `mapstructure:"aws_spend_limit"`
	AzureCreditRemaining string `mapstructure:"azure_credit_remaining"`
	AzureCreditExpires   string `mapstructure:"azure_credit_expires"`
	AWSCreditRemaining   string `mapstructure:"aws_credit_remaining"`
	AWSCreditExpires     string `mapstructure:"aws_credit_expires"`
}

var cfg *Config

func Load(configPath string) (*Config, error) {
	home, _ := os.UserHomeDir()

	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	viper.AddConfigPath(home + "/.azguard")
	viper.AddConfigPath(".")
	viper.AddConfigPath(configPath)

	viper.SetDefault("azure.auth_method", "cli")
	viper.SetDefault("storage.path", "~/.azguard/data.db")

	if err := viper.ReadInConfig(); err != nil {
		_, ok := err.(viper.ConfigFileNotFoundError)
		if !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg.Storage.Path = expandHome(cfg.Storage.Path)
	if err := applySecrets(viper.ConfigFileUsed(), cfg, defaultStore); err != nil {
		return nil, err
	}

	// Auto-detect subscription ID from Azure CLI if not set or invalid
	if cfg.Azure.SubscriptionID == "" {
		if subID, err := azure.GetSubscriptionIDFromCLI(); err == nil {
			cfg.Azure.SubscriptionID = subID
		}
	} else {
		// Validate the subscription ID and try to auto-detect if invalid
		if err := azure.ValidateSubscriptionID(cfg.Azure.SubscriptionID); err != nil {
			// Try to auto-detect from Azure CLI as fallback
			if subID, cliErr := azure.GetSubscriptionIDFromCLI(); cliErr == nil {
				cfg.Azure.SubscriptionID = subID
			}
		}
	}

	return cfg, nil
}

func Get() *Config {
	return cfg
}

func Set(key string, value interface{}) error {
	viper.Set(key, value)
	return viper.WriteConfig()
}

func GetString(key string) string {
	return viper.GetString(key)
}

func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, _ := os.UserHomeDir()
		return home + path[1:]
	}
	if strings.HasPrefix(path, "$HOME") {
		home, _ := os.UserHomeDir()
		return home + strings.TrimPrefix(path, "$HOME")
	}
	return path
}
