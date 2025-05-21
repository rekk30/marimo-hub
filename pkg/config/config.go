package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server struct {
		APIPort    int `mapstructure:"api_port"`
		MarimoPort int `mapstructure:"marimo_port"`
		ProxyPort  int `mapstructure:"proxy_port"`
	} `mapstructure:"server"`
	Notebooks struct {
		Path      string `mapstructure:"path"`
		PortRange struct {
			Start int `mapstructure:"start"`
			End   int `mapstructure:"end"`
		} `mapstructure:"port_range"`
	} `mapstructure:"notebooks"`
	Database struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"database"`
}

var (
	defaults = map[string]interface{}{
		"server.api_port":            8081,
		"server.marimo_port":         8080,
		"server.proxy_port":          80,
		"notebooks.path":             "/notebooks",
		"notebooks.port_range.start": 3000,
		"notebooks.port_range.end":   4000,
		"database.path":              "/data/marimo-hub.db",
	}

	envMappings = map[string]string{
		"API_PORT":            "server.api_port",
		"MARIMO_PORT":         "server.marimo_port",
		"PROXY_PORT":          "server.proxy_port",
		"NOTEBOOKS_PATH":      "notebooks.path",
		"NOTEBOOK_PORT_RANGE": "notebooks.port_range",
		"DB_PATH":             "database.path",
	}
)

func Load() (*Config, error) {
	v := viper.New()

	for key, value := range defaults {
		v.SetDefault(key, value)
	}

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	for env, key := range envMappings {
		if err := v.BindEnv(key, env); err != nil {
			return nil, fmt.Errorf("failed to bind environment variable %s: %w", env, err)
		}
	}

	if portRange := v.GetString("notebooks.port_range"); portRange != "" {
		start, end, err := parsePortRange(portRange)
		if err != nil {
			return nil, fmt.Errorf("invalid port range: %w", err)
		}
		v.Set("notebooks.port_range.start", start)
		v.Set("notebooks.port_range.end", end)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

func parsePortRange(s string) (start, end int, err error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("port range must be in format 'start-end'")
	}

	start, err = parseInt(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start port: %w", err)
	}

	end, err = parseInt(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end port: %w", err)
	}

	return start, end, nil
}

func parseInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func validateConfig(cfg *Config) error {
	if err := validatePort(cfg.Server.APIPort); err != nil {
		return fmt.Errorf("invalid API port: %w", err)
	}
	if err := validatePort(cfg.Server.MarimoPort); err != nil {
		return fmt.Errorf("invalid marimo port: %w", err)
	}
	if err := validatePort(cfg.Server.ProxyPort); err != nil {
		return fmt.Errorf("invalid proxy port: %w", err)
	}

	if err := validatePortRange(cfg.Notebooks.PortRange.Start, cfg.Notebooks.PortRange.End); err != nil {
		return fmt.Errorf("invalid notebook port range: %w", err)
	}

	if !strings.HasPrefix(cfg.Notebooks.Path, "/") {
		return fmt.Errorf("notebooks path must be absolute")
	}
	if !strings.HasPrefix(cfg.Database.Path, "/") {
		return fmt.Errorf("database path must be absolute")
	}

	ports := map[int]string{
		cfg.Server.APIPort:    "API port",
		cfg.Server.MarimoPort: "marimo port",
		cfg.Server.ProxyPort:  "proxy port",
	}
	for port, name := range ports {
		if port == cfg.Notebooks.PortRange.Start || port == cfg.Notebooks.PortRange.End {
			return fmt.Errorf("port conflict: %s (%d) conflicts with notebook port range", name, port)
		}
	}

	return nil
}

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func validatePortRange(start, end int) error {
	if err := validatePort(start); err != nil {
		return err
	}
	if err := validatePort(end); err != nil {
		return err
	}
	if start >= end {
		return fmt.Errorf("start port must be less than end port")
	}
	return nil
}
