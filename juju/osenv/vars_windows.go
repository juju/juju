// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
)

const (
	JujuEnv               = "JUJU_ENV"
	JujuHome              = "JUJU_HOME"
	JujuRepository        = "JUJU_REPOSITORY"
	JujuLxcBridge         = "JUJU_LXC_BRIDGE"
	JujuProviderType      = "JUJU_PROVIDER_TYPE"
	JujuStorageDir        = "JUJU_STORAGE_DIR"
	JujuStorageAddr       = "JUJU_STORAGE_ADDR"
	JujuSharedStorageDir  = "JUJU_SHARED_STORAGE_DIR"
	JujuSharedStorageAddr = "JUJU_SHARED_STORAGE_ADDR"
)

// Home returns the os-specific home path as specified in the environment
func Home() string {
	return path.Join(os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH"))
}

// SetHome sets the os-specific home path in the environment
func SetHome(s string) error {
	v := filepath.VolumeName(s)
	if v != "" {
		if err := os.Setenv("HOMEDRIVE", v); err != nil {
			return err
		}
	}
	return os.Setenv("HOMEPATH", s[len(v):])
}
