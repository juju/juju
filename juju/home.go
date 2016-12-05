// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"path/filepath"

	"github.com/juju/juju/juju/osenv"

	"github.com/juju/errors"
	"github.com/juju/utils/ssh"
	"github.com/juju/utils/winrm"
	"gopkg.in/juju/charmrepo.v2-unstable"
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
	if err := ssh.LoadClientKeys(osenv.JujuXDGDataHomePath("ssh")); err != nil {
		return errors.Annotate(err, "cannot load ssh client keys")
	}

	base := osenv.JujuXDGDataHomePath("x509")
	keyPath := filepath.Join(base, "winrmkey.pem")
	certPath := filepath.Join(base, "winrmcert.crt")
	cert := winrm.NewX509()

	if err := cert.LoadClientCert(keyPath, certPath); err != nil {
		return errors.Annotatef(err, "connot load/create x509 client certs for winrm connection")
	}

	// ignore error because winrm client will default to skip ca verification
	// the user will be alerted in the provisioning process anyway
	ca := filepath.Join(base, "winrmcacert.crt")
	_ = cert.LoadCACert(ca)

	return nil
}
