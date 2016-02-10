// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/juju/utils"
)

// jujuXDGDataHome stores the path to the juju configuration
// folder, which is only meaningful when running the juju
// CLI tool, and is typically defined by $JUJU_DATA or
// $XDG_DATA_HOME/juju or ~/.local/share/juju as default if none
// of the aforementioned variables are defined.
var (
	jujuXDGDataHomeMu sync.Mutex
	jujuXDGDataHome   string
)

// SetJujuXDGDataHome sets the value of juju home and
// returns the current one.
func SetJujuXDGDataHome(newJujuXDGDataHomeHome string) string {
	jujuXDGDataHomeMu.Lock()
	defer jujuXDGDataHomeMu.Unlock()

	oldJujuXDGDataHomeHome := jujuXDGDataHome
	jujuXDGDataHome = newJujuXDGDataHomeHome
	return oldJujuXDGDataHomeHome
}

// JujuXDGDataHome returns the current juju home.
func JujuXDGDataHome() string {
	jujuXDGDataHomeMu.Lock()
	defer jujuXDGDataHomeMu.Unlock()
	if jujuXDGDataHome == "" {
		panic("juju home hasn't been initialized")
	}
	return jujuXDGDataHome
}

// IsJujuXDGDataHomeSet is a way to check if SetJuuHome has been called.
func IsJujuXDGDataHomeSet() bool {
	jujuXDGDataHomeMu.Lock()
	defer jujuXDGDataHomeMu.Unlock()
	return jujuXDGDataHome != ""
}

// JujuXDGDataHomePath returns the path to a file in the
// current juju home.
func JujuXDGDataHomePath(names ...string) string {
	all := append([]string{JujuXDGDataHome()}, names...)
	return filepath.Join(all...)
}

// JujuXDGDataHomeDir returns the directory where juju should store application-specific files
func JujuXDGDataHomeDir() string {
	JujuXDGDataHomeDir := os.Getenv(JujuXDGDataHomeEnvKey)
	if JujuXDGDataHomeDir == "" {
		if runtime.GOOS == "windows" {
			JujuXDGDataHomeDir = jujuXDGDataHomeWin()
		} else {
			JujuXDGDataHomeDir = jujuXDGDataHomeLinux()
		}
	}
	return JujuXDGDataHomeDir
}

// jujuXDGDataHomeLinux returns the directory where juju should store application-specific files on Linux.
func jujuXDGDataHomeLinux() string {
	xdgConfig := os.Getenv(XDGDataHome)
	if xdgConfig != "" {
		return filepath.Join(xdgConfig, "juju")
	}
	// If xdg config home is not defined, the standard indicates that its default value
	// is $HOME/.local/share
	home := utils.Home()
	return filepath.Join(home, ".local/share", "juju")
}

// jujuXDGDataHomeWin returns the directory where juju should store application-specific files on Windows.
func jujuXDGDataHomeWin() string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		return ""
	}
	return filepath.Join(appdata, "Juju")
}
