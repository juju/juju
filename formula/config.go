package formula

import (
	"os"
)

// Option represents a single configuration option that is declared
// as supported by a formula in its config.yaml file.
type Option struct {
	Title       string
	Description string
	Type        string
	Validator   string
	Default     interface{}
}

// Config represents the supported configuration options for a formula,
// as declared in its config.yaml file.
type Config struct {
	Options map[string]Option
}

// ReadConfig reads a config.yaml file and returns its representation.
func ReadConfig(path string) (config *Config, err os.Error) {
	config = &Config{}
	return config, nil
}
