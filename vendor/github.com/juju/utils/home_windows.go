// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"os"
	"path/filepath"
)

// Home returns the os-specific home path as specified in the environment.
func Home() string {
	return filepath.Join(os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH"))
}

// SetHome sets the os-specific home path in the environment.
func SetHome(s string) error {
	v := filepath.VolumeName(s)
	if v != "" {
		if err := os.Setenv("HOMEDRIVE", v); err != nil {
			return err
		}
	}
	return os.Setenv("HOMEPATH", s[len(v):])
}
