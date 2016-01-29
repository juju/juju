// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"github.com/juju/errors"
	"github.com/juju/utils/ssh"
	"gopkg.in/juju/charmrepo.v2-unstable"

	"github.com/juju/juju/juju/osenv"
)

// InitJujuData initializes the charm cache, environs/config and utils/ssh packages
// to use default paths based on the $JUJU_DATA or $HOME environment variables.
// This function should be called before running a Juju CLI command.
func InitJujuData() error {
	jujuHome := osenv.JujuDataDir()
	if jujuHome == "" {
		return errors.New("cannot determine juju home, required environment variables are not set")
	}
	osenv.SetJujuData(jujuHome)
	charmrepo.CacheDir = osenv.JujuDataPath("charmcache")
	if err := ssh.LoadClientKeys(osenv.JujuDataPath("ssh")); err != nil {
		return errors.Annotate(err, "cannot load ssh client keys")
	}
	return nil
}
