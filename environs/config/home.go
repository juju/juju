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
	jujuHomeMu sync.Mutex
	jujuHome   string
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

type FakeJujuHome string

// Restore juju home to the old value.
func (f FakeJujuHome) Restore() {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	jujuHome = string(f)
}

// SetFakeJujuHome allows to set the value of juju home for testing
// purposes.
func SetFakeJujuHome(fake string) FakeJujuHome {
	jujuHomeMu.Lock()
	defer jujuHomeMu.Unlock()

	oldJujuHome := os.Getenv("JUJU_HOME")
	if oldJujuHome == "" {
		// TODO(mue) What if $HOME is unset too?
		oldJujuHome = filepath.Join(os.Getenv("HOME"), ".juju")
	}
	jujuHome = fake
	return FakeJujuHome(oldJujuHome)
}
