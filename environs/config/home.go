package config

import (
	"os"
	"path/filepath"
	"sync"
)

// jujuHome stores the path to the juju configuration
// folder defined by $JUJU_HOME or default ~/.juju.
var (
	jujuHomeMu sync.Mutex
	jujuHome   string
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
// purposes. It returns the current juju home.
func SetTestJujuHome(home string) string {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	current := jujuHome
	jujuHome = home
	return current
}

// RestoreJujuHome (re)initializes the juju home after it may
// have been changed for testing purposes. It returns the
// juju home.
func RestoreJujuHome() string {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	jujuHome = os.Getenv("JUJU_HOME")
	if jujuHome == "" {
		home := os.Getenv("HOME")
		jujuHome = filepath.Join(home, ".juju")
	}
	return jujuHome
}
