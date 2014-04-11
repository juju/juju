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
type StringsValue []string

var _ gnuflag.Value = (*StringsValue)(nil)

// NewStringsValue is used to create the type passed into the gnuflag.FlagSet Var function.
// f.Var(cmd.NewStringsValue(defaultValue, &someMember), "name", "help")
func NewStringsValue(defaultValue []string, target *[]string) *StringsValue {
	value := (*StringsValue)(target)
	*value = defaultValue
	return value
}

// Implements gnuflag.Value Set.
func (v *StringsValue) Set(s string) error {
	*v = strings.Split(s, ",")
	return nil
}

// Implements gnuflag.Value String.
func (v *StringsValue) String() string {
	return strings.Join(*v, ",")
}

// AppendStringsValue implements gnuflag.Value for a value that can be set
// multiple times, and it appends each value to the slice.
type AppendStringsValue []string

var _ gnuflag.Value = (*AppendStringsValue)(nil)

// NewAppendStringsValue is used to create the type passed into the gnuflag.FlagSet Var function.
// f.Var(cmd.NewAppendStringsValue(&someMember), "name", "help")
func NewAppendStringsValue(target *[]string) *AppendStringsValue {
	return (*AppendStringsValue)(target)
}

// Implements gnuflag.Value Set.
func (v *AppendStringsValue) Set(s string) error {
	*v = append(*v, s)
	return nil
}

// Implements gnuflag.Value String.
func (v *AppendStringsValue) String() string {
	return strings.Join(*v, ",")
}
