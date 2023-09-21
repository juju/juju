// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/utils/v3/ssh"

	"github.com/juju/juju/juju/osenv"
)

func checkJujuHomeFolderExists() bool {
	_, err := os.Stat(osenv.JujuXDGDataHomeDir())
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

// InitJujuXDGDataHome initializes the charm cache, environs/config and utils/ssh packages
// to use default paths based on the $JUJU_DATA or $HOME environment variables.
// This function should be called before running a Juju CLI command.
func InitJujuXDGDataHome() error {
	jujuXDGDataHome := osenv.JujuXDGDataHomeDir()
	if jujuXDGDataHome == "" {
		return errors.New("cannot determine juju data home, required environment variables are not set")
	}

	sshDir := osenv.JujuXDGDataHomePath("ssh")
	if err := ssh.LoadClientKeys(sshDir); err != nil {
		// If the ssh directory doesn't exist, we don't want to check that Juju home directory it exists
		if !checkJujuHomeFolderExists() {
			return errors.NewNotFound(err, "cannot create juju home directory")
		} else {
			return errors.Annotate(err, "cannot load ssh client keys")
		}
	}
	ssh.SetGoCryptoKnownHostsFile(filepath.Join(sshDir, "gocrypto_known_hosts"))

	return nil
}
