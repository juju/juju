// Copyright 2011, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The cloudinit package implements a way of creating
// a cloud-init configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/juju/errors"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/version"
)

// Config represents a set of cloud-init configuration options.
type Config struct {
	attrs map[string]interface{}

	// osName is the name of the OS derived from the series. It will
	// be the lower-cased. Currently it only matters if it is
	// "windows" or not "windows".
	osName string

	// Series is the series that this config is targeting.
	Series string

	// ShellRenderer is the shell renderer to use for any commands
	// added to this config.
	ShellRenderer shell.Renderer
}

// New returns a new Config with no options set.
func New(series string) (*Config, error) {
	os, err := version.GetOSFromSeries(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	osName := strings.ToLower(os.String())
	shellRenderer, err := shell.NewRenderer(osName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg := &Config{
		attrs:         make(map[string]interface{}),
		osName:        osName,
		Series:        series,
		ShellRenderer: shellRenderer,
	}
	return cfg, nil
}

func (cfg *Config) set(opt string, yes bool, value interface{}) {
	if yes {
		cfg.attrs[opt] = value
	} else {
		delete(cfg.attrs, opt)
	}
}

// Render converts the cloudinit config into the corresponding script
// to write to disk.
func (cfg *Config) Render() ([]byte, error) {
	if cfg.osName == "windows" {
		return renderWindows(cfg)
	} else {
		return renderUnix(cfg)
	}
}

// AptSource is an apt(8) source, comprising a source location,
// with an optional Key, and optional apt_preferences(5).
type AptSource struct {
	Source string          `yaml:"source"`
	Key    string          `yaml:"key,omitempty"`
	Prefs  *AptPreferences `yaml:"-"`
}

// AptGetWrapper describes a wrapper command for running apt-get.
type AptGetWrapper struct {
	Command string
	Enabled interface{} // true, false or "auto"
}

// AptPreferences is a set of apt_preferences(5) compatible
// preferences for an apt source. It can be used to override the
// default priority for the source. Path where the file will be
// created (usually in /etc/apt/preferences.d/).
type AptPreferences struct {
	Path        string
	Explanation string
	Package     string
	Pin         string
	PinPriority int
}

// FileContents generates an apt_preferences(5) file from the fields
// in prefs.
func (prefs *AptPreferences) FileContents() string {
	const prefTemplate = `
Explanation: {{.Explanation}}
Package: {{.Package}}
Pin: {{.Pin}}
Pin-Priority: {{.PinPriority}}
`
	var buf bytes.Buffer
	t := template.Must(template.New("").Parse(prefTemplate[1:]))
	err := t.Execute(&buf, prefs)
	if err != nil {
		panic(err)
	}
	return buf.String()
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
