// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/cmd"
)

// BootstrapConstraintsFlag records constraints set in bootstrap command
// via bootstrap-constraints flag.
type BootstrapConstraintsFlag []string

// String implements gnuflag.Value.String.
func (c *BootstrapConstraintsFlag) String() string {
	return fmt.Sprintf("%v", *c)
}

// Set implements gnuflag.Value.Set.
func (c *BootstrapConstraintsFlag) Set(value string) error {
	*c = append(*c, value)
	return nil
}

// ConstraintsFlag records constraints set in add-machine, bootstrap, deploy
// commands via constraints flag.
type ConstraintsFlag []string

// String implements gnuflag.Value.String.
func (c *ConstraintsFlag) String() string {
	return fmt.Sprintf("%v", *c)
}

// Set implements gnuflag.Value.Set.
func (c *ConstraintsFlag) Set(value string) error {
	*c = append(*c, value)
	return nil
}

// ConfigFlag records k=v attributes from command arguments
// and/or specified files containing key values.
type ConfigFlag struct {
	files []string
	attrs map[string]interface{}
}

// Set implements gnuflag.Value.Set.
// TODO (stickupkid): Clean this up to correctly handle stdin. Additionally the
// method is confusing and cryptic, we should improve this at some point!
func (f *ConfigFlag) Set(s string) error {
	if s == "" {
		return errors.NotValidf("empty string")
	}

	// Attempt to work out if we're dealing with key value pairs or a string
	// for a file.
	// If the key value pairs don't contain a `=` sign, then it's assumed it's
	// a file, so append that and bail out.
	fields := strings.SplitN(s, "=", 2)
	if len(fields) == 1 {
		f.files = append(f.files, fields[0])
		return nil
	}

	var value interface{} = fields[1]
	if f.attrs == nil {
		f.attrs = make(map[string]interface{})
	}
	f.attrs[fields[0]] = value
	return nil
}

// SetAttrsFromReader sets the attributes from a slice of bytes. The bytes are
// expected to be YAML parsable and align to the attrs type of
// map[string]interface{}.
// This will over write any attributes that already exist if found in the YAML
// configuration.
func (f *ConfigFlag) SetAttrsFromReader(reader io.Reader) error {
	if reader == nil {
		return errors.NotValidf("empty reader")
	}
	attrs := make(map[string]interface{})
	if err := yaml.NewDecoder(reader).Decode(&attrs); err != nil {
		return errors.Trace(err)
	}
	if f.attrs == nil {
		f.attrs = make(map[string]interface{})
	}
	for k, attr := range attrs {
		f.attrs[k] = attr
	}
	return nil
}

// ReadAttrs reads attributes from the specified files, and then overlays
// the results with the k=v attributes.
// TODO (stickupkid): This should only know about io.Readers and correctly
// handle the various path ways from that abstraction.
func (f *ConfigFlag) ReadAttrs(ctx *cmd.Context) (map[string]interface{}, error) {
	attrs := make(map[string]interface{})
	for _, f := range f.files {
		path, err := utils.NormalizePath(f)
		if err != nil {
			return nil, errors.Trace(err)
		}
		data, err := os.ReadFile(ctx.AbsPath(path))
		if err != nil {
			return nil, errors.Trace(err)
		}
		if err := yaml.Unmarshal(data, &attrs); err != nil {
			return nil, err
		}
	}
	for k, v := range f.attrs {
		attrs[k] = v
	}
	return attrs, nil
}

// ReadConfigPairs returns just the k=v attributes.
func (f *ConfigFlag) ReadConfigPairs(ctx *cmd.Context) (map[string]interface{}, error) {
	attrs := make(map[string]interface{})
	for k, v := range f.attrs {
		attrs[k] = v
	}
	return attrs, nil
}

// AbsoluteFileNames returns the absolute path of any file names specified.
func (f *ConfigFlag) AbsoluteFileNames(ctx *cmd.Context) ([]string, error) {
	files := make([]string, len(f.files))
	for i, f := range f.files {
		path, err := utils.NormalizePath(f)
		if err != nil {
			return nil, errors.Trace(err)
		}
		files[i] = ctx.AbsPath(path)
	}
	return files, nil
}

// String implements gnuflag.Value.String.
func (f *ConfigFlag) String() string {
	strs := make([]string, 0, len(f.attrs)+len(f.files))
	strs = append(strs, f.files...)
	for k, v := range f.attrs {
		strs = append(strs, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(strs, " ")
}

// WarnConstraintAliases shows a warning to the user that they have used an
// alias for a constraint that might go away sometime.
func WarnConstraintAliases(ctx *cmd.Context, aliases map[string]string) {
	for alias, canonical := range aliases {
		ctx.Infof("Warning: constraint %q is deprecated in favor of %q.\n", alias, canonical)
	}
}

// ParseConstraints parses the given constraints and uses WarnConstraintAliases
// if any aliases were used.
func ParseConstraints(ctx *cmd.Context, cons string) (constraints.Value, error) {
	if cons == "" {
		return constraints.Value{}, nil
	}
	constraint, aliases, err := constraints.ParseWithAliases(cons)
	// we always do these, even on errors, so that the error messages have
	// context.
	for alias, canonical := range aliases {
		ctx.Infof("Warning: constraint %q is deprecated in favor of %q.\n", alias, canonical)
	}
	if err != nil {
		return constraints.Value{}, err
	}
	return constraint, nil
}

// AutoBoolValue is like gnuflag.boolValue, but remembers
// whether or not a value has been set, so its behaviour
// can be determined dynamically, during command execution.
type AutoBoolValue struct {
	b *bool
}

// Set implements flag.Value.
func (b *AutoBoolValue) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	b.b = &v
	return nil
}

// Get returns the value.
func (b *AutoBoolValue) Get() *bool {
	return b.b
}

// String implements flag.Value.
func (b *AutoBoolValue) String() string {
	if b.b != nil {
		return fmt.Sprint(*b.b)
	}
	return "nil"
}

// IsBoolFlag implements flag.Value.
func (b *AutoBoolValue) IsBoolFlag() bool { return true }
