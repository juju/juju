// Copyright 2011, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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

// AptSource is an apt(8) source, comprising a source location,
// optionally either a Key, or KeyId with an optional KeyServer.
type AptSource struct {
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
