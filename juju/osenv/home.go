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

// jujuHome stores the path to the juju configuration
// folder, which is only meaningful when running the juju
// CLI tool, and is typically defined by $JUJU_DATA or
// $XDG_DATA_HOME/juju or ~/.local/share/juju as default if none
// of the aforementioned variables are defined.
var (
	jujuHomeMu sync.Mutex
	jujuHome   string
)

// SetJujuData sets the value of juju home and
// returns the current one.
func SetJujuData(newJujuData string) string {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	oldJujuData := jujuHome
	jujuHome = newJujuData
	return oldJujuData
}

// JujuData returns the current juju home.
func JujuData() string {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()
	if jujuHome == "" {
		panic("juju home hasn't been initialized")
	}
	return jujuHome
}

// IsJujuDataSet is a way to check if SetJuuHome has been called.
func IsJujuDataSet() bool {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()
	return jujuHome != ""
}

// JujuDataPath returns the path to a file in the
// current juju home.
func JujuDataPath(names ...string) string {
	all := append([]string{JujuData()}, names...)
	return filepath.Join(all...)
}

// JujuDataDir returns the directory where juju should store application-specific files
func JujuDataDir() string {
	JujuDataDir := os.Getenv(JujuDataEnvKey)
	if JujuDataDir == "" {
		if runtime.GOOS == "windows" {
			JujuDataDir = jujuHomeWin()
		} else {
			JujuDataDir = jujuHomeLinux()
		}
	}
	return JujuDataDir
}

// jujuHomeLinux returns the directory where juju should store application-specific files on Linux.
func jujuHomeLinux() string {
	xdgConfig := os.Getenv(XDGDataHome)
	if xdgConfig != "" {
		return filepath.Join(xdgConfig, "juju")
	}
	// If xdg config home is not defined, the standard indicates that its default value
	// is $HOME/.local/share
	home := utils.Home()
	return filepath.Join(home, ".local/share", "juju")
}

// jujuHomeWin returns the directory where juju should store application-specific files on Windows.
func jujuHomeWin() string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		return ""
	}
	return filepath.Join(appdata, "Juju")
}
