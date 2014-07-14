// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	stderrors "errors"
	"fmt"

	"github.com/juju/charm"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/utils/ssh"
)

// InitJujuHome initializes the charm cache, environs/config and utils/ssh packages
// to use default paths based on the $JUJU_HOME or $HOME environment variables.
// This function should be called before running a Juju CLI command.
func InitJujuHome() error {
	jujuHome := osenv.JujuHomeDir()
	if jujuHome == "" {
		return stderrors.New(
			"cannot determine juju home, required environment variables are not set")
	}
	osenv.SetJujuHome(jujuHome)
	charm.CacheDir = osenv.JujuHomePath("charmcache")
	if err := ssh.LoadClientKeys(osenv.JujuHomePath("ssh")); err != nil {
		return fmt.Errorf("cannot load ssh client keys: %v", err)
	}
	return nil
}
