// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/constraints"
	"github.com/juju/utils"
	"gopkg.in/yaml.v2"
)

// ConfigFlag records k=v attributes from command arguments
// and/or specified files containing key values.
type ConfigFlag struct {
	files []string
	attrs map[string]interface{}
}

// Set implements gnuflag.Value.Set.
func (f *ConfigFlag) Set(s string) error {
	if s == "" {
		return errors.NotValidf("empty string")
	}
	fields := strings.SplitN(s, "=", 2)
	if len(fields) == 1 {
		f.files = append(f.files, fields[0])
		return nil
	}
	var value interface{}
	if err := yaml.Unmarshal([]byte(fields[1]), &value); err != nil {
		return errors.Trace(err)
	}
	if f.attrs == nil {
		f.attrs = make(map[string]interface{})
	}
	f.attrs[fields[0]] = value
	return nil
}

// ReadAttrs reads attributes from the specified files, and then overlays
// the results with the k=v attributes.
func (f *ConfigFlag) ReadAttrs(ctx *cmd.Context) (map[string]interface{}, error) {
	attrs := make(map[string]interface{})
	for _, f := range f.files {
		path, err := utils.NormalizePath(f)
		if err != nil {
			return nil, errors.Trace(err)
		}
		data, err := ioutil.ReadFile(ctx.AbsPath(path))
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

// String implements gnuflag.Value.String.
func (f *ConfigFlag) String() string {
	strs := make([]string, 0, len(f.attrs)+len(f.files))
	for _, f := range f.files {
		strs = append(strs, f)
	}
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
