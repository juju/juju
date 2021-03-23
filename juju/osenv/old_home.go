// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/juju/utils/v2"
)

// Juju1xEnvConfigExists returns true if there is an environments.yaml file in
// the expected juju 1.x directory.
func Juju1xEnvConfigExists() bool {
	dir := OldJujuHomeDir()
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "environments.yaml"))
	return err == nil
}

// The following code is copied from juju 1.x, only the names have been changed
// to protect the innocent.

// oldJujuHomeEnvKey holds the environment variable that a user could set to
// override where juju 1.x stored application data.
const oldJujuHomeEnvKey = "JUJU_HOME"

// OldJujuHomeDir returns the directory where juju 1.x stored
// application-specific files.
func OldJujuHomeDir() string {
	JujuHomeDir := os.Getenv(oldJujuHomeEnvKey)
	if JujuHomeDir == "" {
		if runtime.GOOS == "windows" {
			JujuHomeDir = oldJujuHomeWin()
		} else {
			JujuHomeDir = oldJujuHomeLinux()
		}
	}
	return JujuHomeDir
}

// oldJujuHomeLinux returns the directory where juju 1.x stored
// application-specific files on Linux.
func oldJujuHomeLinux() string {
	home := utils.Home()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".juju")
}

// oldJujuHomeWin returns the directory where juju 1.x stored
// application-specific files on Windows.
func oldJujuHomeWin() string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		return ""
	}
	return filepath.Join(appdata, "Juju")
}
