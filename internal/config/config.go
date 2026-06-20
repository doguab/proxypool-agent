package config

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const DefaultPath = "/etc/proxypool/agent.yaml"

type Config struct {
	HubURL         string `yaml:"hub_url"`
	Secret         string `yaml:"secret"`
	MaxConnections int    `yaml:"max_connections"`
}

func Default() Config {
	return Config{
		HubURL:         "",
		Secret:         "",
		MaxConnections: 50,
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.MaxConnections <= 0 {
		cfg.MaxConnections = 50
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func GenerateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	out := make([]byte, 52)
	for i := range out {
		out[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(out), nil
}

func EnsureSecret(cfg *Config) (bool, error) {
	if cfg.Secret != "" {
		return false, nil
	}
	secret, err := GenerateSecret()
	if err != nil {
		return false, err
	}
	cfg.Secret = secret
	return true, nil
}

func Validate(cfg Config) error {
	if cfg.HubURL == "" {
		return fmt.Errorf("hub_url is required")
	}
	if cfg.Secret == "" {
		return fmt.Errorf("secret is required")
	}
	return nil
}
