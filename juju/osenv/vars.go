// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package osenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	JujuEnv           = "JUJU_ENV"
	JujuHome          = "JUJU_HOME"
	JujuRepository    = "JUJU_REPOSITORY"
	JujuLoggingConfig = "JUJU_LOGGING_CONFIG"
	// TODO(thumper): 2013-09-02 bug 1219630
	// As much as I'd like to remove JujuContainerType now, it is still
	// needed as MAAS still needs it at this stage, and we can't fix
	// everything at once.
	JujuContainerType = "JUJU_CONTAINER_TYPE"
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

// ProxySettings holds the values for the http, https and ftp proxies found by
// Detect Proxies.
type ProxySettings struct {
	Http  string
	Https string
	Ftp   string
}

func getProxySetting(key string) string {
	value := os.Getenv(key)
	if value == "" {
		value = os.Getenv(strings.ToUpper(key))
	}
	return value
}

// DetectProxies returns the proxy settings found the environment.
func DetectProxies() ProxySettings {
	return ProxySettings{
		Http:  getProxySetting("http_proxy"),
		Https: getProxySetting("https_proxy"),
		Ftp:   getProxySetting("ftp_proxy"),
	}
}
