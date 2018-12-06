// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

type regionsFlag struct {
	regions *[]string
}

// Set implements gnuflag.Value.Set.
func (f regionsFlag) Set(s string) error {
	return nil
}

// String implements gnuflag.Value.String.
func (f regionsFlag) String() string {
	return ""
}
