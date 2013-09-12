// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	JujuEnv               = "JUJU_ENV"
	JujuHome              = "JUJU_HOME"
	JujuRepository        = "JUJU_REPOSITORY"
	JujuLxcBridge         = "JUJU_LXC_BRIDGE"
	JujuProviderType      = "JUJU_PROVIDER_TYPE"
	JujuContainerType     = "JUJU_CONTAINER_TYPE"
	JujuStorageDir        = "JUJU_STORAGE_DIR"
	JujuStorageAddr       = "JUJU_STORAGE_ADDR"
	JujuSharedStorageDir  = "JUJU_SHARED_STORAGE_DIR"
	JujuSharedStorageAddr = "JUJU_SHARED_STORAGE_ADDR"
)

// JujuHome returns the directory where juju should store application-specific files
func JujuHomeDir() string {
	jujuHome := os.Getenv(JujuHome)
	if jujuHome == "" {
		if runtime.GOOS == "windows" {
			jujuHome = jujuHomeWin()
		} else {
			jujuHome = jujuHomeLinux()
		}
	}
	return jujuHome
}

// jujuHomeLinux returns the directory where juju should store application-specific files on Linux.
func jujuHomeLinux() string {
	home := Home()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".juju")
}

// jujuHomeWin returns the directory where juju should store application-specific files on Windows.
func jujuHomeWin() string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		return ""
	}
	return filepath.Join(appdata, "Juju")
}
