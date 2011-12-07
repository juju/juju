// The cloudinit package implements a way of creating
// a cloudinit configuration file.
// See https://help.ubuntu.com/community/CloudInit.
package cloudinit

import (
	"fmt"
	yaml "launchpad.net/goyaml"
	"reflect"
)

// Config represents a set of cloud-init configuration options.
type Config struct {
	attrs map[string]interface{}
}

// New returns a new Config with no options set.
func New() *Config {
	return &Config{make(map[string]interface{})}
}

// Render returns the cloudinit configuration as a YAML file.
func (cfg *Config) Render() ([]byte, error) {
	data, err := yaml.Marshal(cfg.attrs)
	if err != nil {
		return nil, err
	}
	return append([]byte("#cloud-config\n"), data...), nil
}

// Option represents a cloudinit configuration option.
// If it is added to a Config, Name and Value will be marshalled as a top level
// attribute-value pair in the generated YAML.
type Option struct {
	Name  string
	Value interface{}
}

// Add sets the given configuration option in cfg.
func (cfg *Config) Set(opt Option) {
	if opt.Value != nil {
		cfg.attrs[opt.Name] = opt.Value
	}
}

// Append appends the option's value to the existing value for
// that option in cfg. The value must be of slice type.
func (cfg *Config) Append(opt Option) {
	if opt.Value == nil {
		return
	}
	old := cfg.attrs[opt.Name]
	if old == nil {
		cfg.attrs[opt.Name] = opt.Value
		return
	}
	v := reflect.ValueOf(opt.Value)
	if v.Kind() != reflect.Slice {
		panic(fmt.Errorf("cloudinit.Config.Append given option (%s) of non-slice type", opt.Name))
	}
	oldv := reflect.ValueOf(old)
	if v.Type() != oldv.Type() {
		panic(fmt.Errorf("cloudinit.Config.Append: mismatched type, expected %v got %v", oldv.Type(), v.Type()))
	}

	cfg.attrs[opt.Name] = reflect.AppendSlice(oldv, v).Interface()
}

// source is Key, or KeyId and KeyServer
type source struct {
	Source    string `yaml:"source"`
	Key       string `yaml:"key,omitempty"`
	KeyId     string `yaml:"keyid,omitempty"`
	KeyServer string `yaml:"keyserver,omitempty"`
}

// Source represents a source option to AptSources.
type Source struct {
	source source
}

// NewSource creates a Source with the given name from a key.
func NewSource(name string, key string) *Source {
	return &Source{source: source{
		Source: name,
		Key:    key,
	}}
}

// NewSource creates a Source with the given name from a key id
// and a key server.
func NewSourceWithKeyId(name, keyId, keyServer string) *Source {
	return &Source{source: source{
		Source:    name,
		KeyId:     keyId,
		KeyServer: keyServer,
	}}
}

// Command represents a shell command.
type Command struct {
	literal string
	args    []string
}

// NewLiteralCommand returns a Command which
// will run s as a shell command. Shell metacharacters
// in s will be interpreted by the shell.
func NewLiteralCommand(s string) *Command {
	return &Command{literal: s}
}

// NewArgListCommand returns a Command which
// run the given command and arguments. Any
// shell metacharacters in the arguments will be
// appropriately quoted.
func NewArgListCommand(args ...string) *Command {
	return &Command{args: args}
}

// GetYAML implements yaml.Getter
func (t *Command) GetYAML() (tag string, value interface{}) {
	if t.args != nil {
		return "", t.args
	}
	return "", t.literal
}

// KeyType represents the type of an SSH Key.
type KeyType int

const (
	_ KeyType = iota
	RSA
	DSA

	Private KeyType = 1 << 3
	Public  KeyType = 0 << 3

	RSAPrivate = RSA | Private
	RSAPublic  = RSA | Public
	DSAPrivate = DSA | Private
	DSAPublic  = DSA | Public
)

var _ yaml.Getter = Key{}

// Key represents an SSH Key with the given type and associated key data.
type Key struct {
	Type KeyType
	Data string
}

// GetYaml implements yaml.Getter
func (k Key) GetYAML() (tag string, value interface{}) {
	return "", []string{k.Type.String(), k.Data}
}

func (t KeyType) String() string {
	var s string
	switch t &^ (Private | Public) {
	case RSA:
		s = "rsa"
	case DSA:
		s = "dsa"
	default:
		panic("unknown key type")
	}
	if t&Private != 0 {
		s += "_private"
	} else {
		s += "_public"
	}
	return s
}

// OutputSpec represents the destination of a command.
// Each of Stdout and Stderr can take one of the following forms:
// >>file
//	appends to file
// >file
//	overwrites file
// |command
//	pipes to the given command.
// If Stderr is "&1", it will be directed to the same
// place as Stdout.
type OutputSpec struct {
	Stdout string
	Stderr string
}

var _ yaml.Getter = (*OutputSpec)(nil)

func (o *OutputSpec) GetYAML() (tag string, value interface{}) {
	if o.Stdout == o.Stderr {
		return "", o.Stdout
	}
	return "", []string{o.Stdout, o.Stderr}
}

// maybe returns x if yes is true, otherwise it returns nil.
func maybe(yes bool, x interface{}) interface{} {
	if yes {
		return x
	}
	return nil
}
