// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

type optionFlag struct {
	options *map[string]interface{}
}

// Set implements gnuflag.Value.Set.
func (f optionFlag) Set(s string) error {
	fields := strings.SplitN(s, "=", 2)
	if len(fields) < 2 {
		return errors.New("expected <key>=<value>")
	}
	var value interface{}
	if err := yaml.Unmarshal([]byte(fields[1]), &value); err != nil {
		return errors.Trace(err)
	}
	if *f.options == nil {
		*f.options = make(map[string]interface{})
	}
	(*f.options)[fields[0]] = value
	return nil
}

// String implements gnuflag.Value.String.
func (f optionFlag) String() string {
	strs := make([]string, 0, len(*f.options))
	for k, v := range *f.options {
		strs = append(strs, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(strs, " ")
}
