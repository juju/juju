// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"github.com/juju/errors"
	"github.com/juju/utils/ssh"
	"gopkg.in/juju/charmrepo.v2-unstable"

	"github.com/juju/juju/juju/osenv"
)

// InitJujuHome initializes the charm cache, environs/config and utils/ssh packages
// to use default paths based on the $JUJU_HOME or $HOME environment variables.
// This function should be called before running a Juju CLI command.
func InitJujuHome() error {
	jujuHome := osenv.JujuHomeDir()
	if jujuHome == "" {
		return errors.New("cannot determine juju home, required environment variables are not set")
	}
	osenv.SetJujuHome(jujuHome)
	charmrepo.CacheDir = osenv.JujuHomePath("charmcache")
	if err := ssh.LoadClientKeys(osenv.JujuHomePath("ssh")); err != nil {
		return errors.Annotate(err, "cannot load ssh client keys")
	}
	return nil
}
