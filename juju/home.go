// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"path/filepath"

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
		return errors.New("cannot determine juju data home, required environment variables are not set")
	}
	charmrepo.CacheDir = osenv.JujuXDGDataHomePath("charmcache")

	sshDir := osenv.JujuXDGDataHomePath("ssh")
	if err := ssh.LoadClientKeys(sshDir); err != nil {
		return errors.Annotate(err, "cannot load ssh client keys")
	}
	ssh.SetGoCryptoKnownHostsFile(filepath.Join(sshDir, "gocrypto_known_hosts"))

	return nil
}
