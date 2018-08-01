// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"strings"
)

// Setenv sets an environment variable entry in the given env slice (as
// returned by os.Environ or passed in exec.Cmd.Environ) to the given
// value. The entry should be in the form "x=y" where x is the name of the
// environment variable and y is its value; if not, env will be
// returned unchanged.
//
// If a value isn't already present in the slice, the entry is appended.
//
// The new environ slice is returned.
func Setenv(env []string, entry string) []string {
	i := strings.Index(entry, "=")
	if i == -1 {
		return env
	}
	prefix := entry[0 : i+1]
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = entry
			return env
		}
	}
	return append(env, entry)
}
