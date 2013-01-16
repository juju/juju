// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"bytes"
	"compress/gzip"
	"fmt"
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

// Render returns the cloud-init configuration as gzip-compressed YAML file.
func (cfg *Config) RenderCompressed() ([]byte, error) {
	data, err := cfg.Render()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, fmt.Errorf("cannot compress data: %v", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("cannot compress data: %v", err)
	}
	return buf.Bytes(), nil
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

type SSHKeyType string

const (
	RSAPrivate SSHKeyType = "rsa_private"
	RSAPublic  SSHKeyType = "rsa_public"
	DSAPrivate SSHKeyType = "dsa_private"
	DSAPublic  SSHKeyType = "dsa_public"
)
