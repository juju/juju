// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"github.com/juju/errors"
	"github.com/juju/utils/ssh"
	"gopkg.in/juju/charmrepo.v2-unstable"

	"github.com/juju/juju/juju/osenv"
)

// InitJujuXDGDataHome initializes the charm cache, environs/config and utils/ssh packages
// to use default paths based on the $JUJU_DATA or $HOME environment variables.
// This function should be called before running a Juju CLI command.
func InitJujuXDGDataHome() error {
	jujuXDGDataHome := osenv.JujuXDGDataHomeDir()
	if jujuXDGDataHome == "" {
		return errors.New("cannot determine juju data home, required model variables are not set")
	}
	osenv.SetJujuXDGDataHome(jujuXDGDataHome)
	charmrepo.CacheDir = osenv.JujuXDGDataHomePath("charmcache")
	if err := ssh.LoadClientKeys(osenv.JujuXDGDataHomePath("ssh")); err != nil {
		return errors.Annotate(err, "cannot load ssh client keys")
	}
	return nil
}
