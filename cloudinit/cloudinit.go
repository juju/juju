// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	yaml "launchpad.net/goyaml"
)

// Config represents a set of cloud-init configuration options.
type Config struct {
	attrs map[string]interface{}
}

// New returns a new Config with no options set.
func New() *Config {
	return &Config{make(map[string]interface{})}
}

// Render returns the cloud-init configuration as a YAML file.
func (cfg *Config) Render() ([]byte, error) {
	data, err := yaml.Marshal(cfg.attrs)
	if err != nil {
		return nil, err
	}
	return append([]byte("#cloud-config\n"), data...), nil
}

func (cfg *Config) set(opt string, yes bool, value interface{}) {
	if yes {
		cfg.attrs[opt] = value
	} else {
		delete(cfg.attrs, opt)
	}
}

// source is Key, or KeyId and KeyServer
type source struct {
	Source    string `yaml:"source"`
	Key       string `yaml:"key,omitempty"`
	KeyId     string `yaml:"keyid,omitempty"`
	KeyServer string `yaml:"keyserver,omitempty"`
}

// command represents a shell command.
type command struct {
	literal string
	args    []string
}

// GetYAML implements yaml.Getter
func (t *command) GetYAML() (tag string, value interface{}) {
	if t.args != nil {
		return "", t.args
	}
	return "", t.literal
}

// Alg represents a possible SSH key type.
type Alg uint
const (
	RSA Alg = iota
	DSA
)

// key represents an SSH Key with the given type and associated key data.
type key struct {
	alg Alg
	private bool
	data    string
}

var algNames = []string {
	RSA: "rsa",
	DSA: "dsa",
}

var _ yaml.Getter = key{}

// GetYaml implements yaml.Getter
func (k key) GetYAML() (tag string, value interface{}) {
	s := algNames[k.alg]
	if k.private {
		s += "_private"
	} else {
		s += "_public"
	}
	return "", []string{s, k.data}
}
