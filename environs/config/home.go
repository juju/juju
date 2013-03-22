package config

import (
	"errors"
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

// Init retrieves $JUJU_HOME or $HOME to set the juju home.
// In case both variables aren't set an error is returned. 
func Init() error {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	jujuHome = os.Getenv("JUJU_HOME")
	if jujuHome == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return errors.New("cannot determine juju home, neither $JUJU_HOME nor $HOME are set")
		}
		jujuHome = filepath.Join(home, ".juju")
	}
	if jujuHomeOrig == "" {
		// Store the original juju home only once.
		jujuHomeOrig = jujuHome
	}
	return nil
}

// JujuHome returns the current juju home.
func JujuHome() string {
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

// SetTestJujuHome allows to set the value of juju home for test
// purposes. It returns the original juju home.
func SetTestJujuHome(home string) string {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	if jujuHomeOrig == "" {
		panic("juju home hasn't been initialized")
	}
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

	if jujuHomeOrig == "" {
		panic("juju home hasn't been initialized")
	}
	jujuHome = jujuHomeOrig
	os.Setenv("JUJU_HOME", jujuHome)
	return jujuHome
}
