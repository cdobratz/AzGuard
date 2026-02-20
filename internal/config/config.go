package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Ollama    OllamaConfig    `mapstructure:"ollama"`
	Anthropic AnthropicConfig  `mapstructure:"anthropic"`
	Azure     AzureConfig     `mapstructure:"azure"`
	AWS       AWSConfig       `mapstructure:"aws"`
	GCP       GCPConfig       `mapstructure:"gcp"`
	Storage   StorageConfig   `mapstructure:"storage"`
}

type OllamaConfig struct {
	BaseURL string `mapstructure:"base_url"`
	Model   string `mapstructure:"model"`
}

type AnthropicConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
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

var cfg *Config

func Load(configPath string) (*Config, error) {
	home, _ := os.UserHomeDir()
	
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	viper.AddConfigPath(home + "/.azguard")
	viper.AddConfigPath(".")
	viper.AddConfigPath(configPath)

	viper.SetDefault("ollama.base_url", "http://localhost:11434")
	viper.SetDefault("ollama.model", "codellama")
	viper.SetDefault("anthropic.model", "claude-3-sonnet-20240229")
	viper.SetDefault("azure.auth_method", "cli")
	viper.SetDefault("storage.path", "~/.azguard/data.db")

	envFile := os.Getenv("AGENT_ENV_FILE")
	if envFile != "" {
		viper.SetConfigFile(envFile)
		if err := viper.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read env file: %w", err)
		}
	}

	if err := viper.BindEnv("anthropic.api_key", "ANTHROPIC_API_KEY"); err != nil {
		return nil, err
	}
	if err := viper.BindEnv("azure.client_secret", "AZURE_CLIENT_SECRET"); err != nil {
		return nil, err
	}

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
	cfg.Ollama.BaseURL = expandHome(cfg.Ollama.BaseURL)

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
		return home + path[5:]
	}
	return path
}
