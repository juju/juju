// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"path/filepath"
	"sync"
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
