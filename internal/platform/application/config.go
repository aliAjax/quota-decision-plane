package application

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address         string
	NodeID          string
	APIKey          string
	RequestTimeout  time.Duration
	MaxBodyBytes    int64
	RateLimit       int
	ReservationReap time.Duration
	LogLevel        string
}

func DefaultConfig() Config {
	return Config{Address: ":18333", NodeID: "quota-node-1", APIKey: "dev-secret", RequestTimeout: 5 * time.Second, MaxBodyBytes: 1 << 20, RateLimit: 200, ReservationReap: time.Second, LogLevel: "info"}
}
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return cfg, fmt.Errorf("open config: %w", err)
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key, value := strings.TrimSpace(parts[0]), strings.Trim(strings.TrimSpace(parts[1]), "\"")
			if err := assignConfig(&cfg, key, value); err != nil {
				return cfg, fmt.Errorf("config %s: %w", key, err)
			}
		}
		if err := scanner.Err(); err != nil {
			return cfg, fmt.Errorf("scan config: %w", err)
		}
	}
	env := map[string]*string{"QUOTA_ADDRESS": &cfg.Address, "QUOTA_NODE_ID": &cfg.NodeID, "QUOTA_API_KEY": &cfg.APIKey, "QUOTA_LOG_LEVEL": &cfg.LogLevel}
	for key, target := range env {
		if value, ok := os.LookupEnv(key); ok {
			*target = value
		}
	}
	if value, ok := os.LookupEnv("QUOTA_REQUEST_TIMEOUT"); ok {
		d, err := time.ParseDuration(value)
		if err != nil {
			return cfg, err
		}
		cfg.RequestTimeout = d
	}
	if value, ok := os.LookupEnv("QUOTA_MAX_BODY_BYTES"); ok {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return cfg, err
		}
		cfg.MaxBodyBytes = n
	}
	if value, ok := os.LookupEnv("QUOTA_RATE_LIMIT"); ok {
		n, err := strconv.Atoi(value)
		if err != nil {
			return cfg, err
		}
		cfg.RateLimit = n
	}
	return cfg, validateConfig(cfg)
}
func assignConfig(cfg *Config, key, value string) error {
	switch key {
	case "address":
		cfg.Address = value
	case "node_id":
		cfg.NodeID = value
	case "api_key":
		cfg.APIKey = value
	case "log_level":
		cfg.LogLevel = value
	case "request_timeout":
		d, e := time.ParseDuration(value)
		if e != nil {
			return e
		}
		cfg.RequestTimeout = d
	case "max_body_bytes":
		n, e := strconv.ParseInt(value, 10, 64)
		if e != nil {
			return e
		}
		cfg.MaxBodyBytes = n
	case "rate_limit":
		n, e := strconv.Atoi(value)
		if e != nil {
			return e
		}
		cfg.RateLimit = n
	case "reservation_reap_interval":
		d, e := time.ParseDuration(value)
		if e != nil {
			return e
		}
		cfg.ReservationReap = d
	}
	return nil
}
func validateConfig(c Config) error {
	if c.Address == "" || c.NodeID == "" {
		return fmt.Errorf("address and node_id required")
	}
	if c.RequestTimeout <= 0 || c.MaxBodyBytes < 1024 || c.RateLimit < 1 {
		return fmt.Errorf("invalid timeout, body limit, or rate limit")
	}
	return nil
}
