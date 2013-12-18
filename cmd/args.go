// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"strings"

	"launchpad.net/gnuflag"
)

// StringsValue implements gnuflag.Value for a comma separated list of
// strings.  This allows flags to be created where the target is []string, and
// the caller is after comma separated values.
type StringsValue struct {
	Target *[]string
}

var _ gnuflag.Value = (*StringsValue)(nil)

func NewStringsValue(defaultValue []string, target *[]string) *StringsValue {
	value := StringsValue{target}
	*value.Target = defaultValue
	return &value
}

func (v *StringsValue) Set(s string) error {
	parts := strings.Split(s, ",")
	var result []string
	for _, v := range parts {
		trimmed := strings.TrimSpace(v)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	*v.Target = result
	return nil
}

func (v *StringsValue) String() string {
	return strings.Join(*v.Target, ",")
}
