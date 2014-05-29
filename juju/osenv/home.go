// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"launchpad.net/juju-core/utils"
)

// jujuHome stores the path to the juju configuration
// folder, which is only meaningful when running the juju
// CLI tool, and is typically defined by $JUJU_HOME or
// $HOME/.juju as default.
var (
	jujuHomeMu sync.Mutex
	jujuHome   string
)

// SetJujuHome sets the value of juju home and
// returns the current one.
func SetJujuHome(newJujuHome string) string {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	oldJujuHome := jujuHome
	jujuHome = newJujuHome
	return oldJujuHome
}

// JujuHome returns the current juju home.
func JujuHome() string {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()
	if jujuHome == "" {
		panic("juju home hasn't been initialized")
	}
	return jujuHome
}

// JujuHomePath returns the path to a file in the
// current juju home.
func JujuHomePath(names ...string) string {
	all := append([]string{JujuHome()}, names...)
	return filepath.Join(all...)
}

// JujuHomeDir returns the directory where juju should store application-specific files
func JujuHomeDir() string {
	JujuHomeDir := os.Getenv(JujuHomeEnvKey)
	if JujuHomeDir == "" {
		if runtime.GOOS == "windows" {
			JujuHomeDir = jujuHomeWin()
		} else {
			JujuHomeDir = jujuHomeLinux()
		}
	}
	return JujuHomeDir
}

// jujuHomeLinux returns the directory where juju should store application-specific files on Linux.
func jujuHomeLinux() string {
	home := utils.Home()
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
