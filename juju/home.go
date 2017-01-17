// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"os"
	"time"
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/ssh"
	"gopkg.in/juju/charmrepo.v2-unstable"

	"github.com/juju/juju/cert"
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
	if err := ssh.LoadClientKeys(osenv.JujuXDGDataHomePath("ssh")); err != nil {
		return errors.Annotate(err, "cannot load ssh client keys")
	}
	expiry := time.Now().UTC().AddDate(10, 0, 0)
	uuid, _ := utils.NewUUID()
	certdir := osenv.JujuXDGDataHomePath("x509")
	if certdir != "" {
		err := os.MkdirAll(certdir, 0700)
		if err == nil {
			pem, key, err := cert.NewCA("client", uuid.String(), expiry)
			public, err := cert.ComputePublicKey(pem)
			err = ioutil.WriteFile(certdir + "/juju-cert.pem", []byte(pem), 0600)
			err = ioutil.WriteFile(certdir + "/juju-cert.key", []byte(key), 0600)
			err = ioutil.WriteFile(certdir + "/juju-cert.pub", []byte(public), 0600)
			if err != nil {
				return errors.Annotate(err, "cannot write x509 certificate")
			}
		}
	}
	return nil
}
