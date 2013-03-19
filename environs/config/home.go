package config

import (
	"os"
	"path/filepath"
	"sync"
)

// jujuHome stores the path to the juju configuration
// folder defined by $JUJU_HOME or default ~/.juju.
var (
	jujuHomeMu   sync.Mutex
	jujuHome     string
	jujuHomeOrig string
)

func init() {
	RestoreJujuHome()
}

// JujuHome returns the current juju home.
func JujuHome() string {
	return jujuHome
}

// JujuHomePath returns the path to a file in the
// current juju home.
func JujuHomePath(names ...string) string {
	all := append([]string{jujuHome}, names...)
	return filepath.Join(all...)
}

// SetTestJujuHome allows to set the value of juju home for test
// purposes. It returns the original juju home.
func SetTestJujuHome(home string) string {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	jujuHome = home
	os.Setenv("JUJU_HOME", jujuHome)
	return jujuHomeOrig
}

// RestoreJujuHome (re)initializes the juju home after it may
// have been changed for testing purposes. It returns the
// juju home.
func RestoreJujuHome() string {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	if jujuHomeOrig != "" {
		// Restore the original juju home.
		jujuHome = jujuHomeOrig
	} else {
		// Retrieve juju home either by the environment variable
		// of derived from the home environment variable.
		jujuHome = os.Getenv("JUJU_HOME")
		if jujuHome == "" {
			home := os.Getenv("HOME")
			if home == "" {
				panic("environs/config: neither $JUJU_HOME nor $HOME are set")
			}
			jujuHome = filepath.Join(home, ".juju")
		}
	}
	if jujuHomeOrig == "" {
		// Store the original juju home only once.
		jujuHomeOrig = jujuHome
	}
	os.Setenv("JUJU_HOME", jujuHome)
	return jujuHome
}
