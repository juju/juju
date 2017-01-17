// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/ssh"
	"github.com/juju/utils/cert"
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
	if err := ssh.LoadClientKeys(osenv.JujuXDGDataHomePath("ssh")); err != nil {
		return errors.Annotate(err, "cannot load ssh client keys")
	}
	if err := InitJujuX509Certificate(); err != nil{
		return errors.Annotate(err, "cannot create x509 certificate")
	}
	return nil
}

// InitJujuX509Certificate initializes a default x509 certificate to be stored
// in the config folder this certificate can be used by any provider that requires
// encryption through this kind of certificates, for instance, LXD.
func InitJujuX509Certificate() (error) {
	expiry := time.Now().UTC().AddDate(10, 0, 0)
	uuid, _ := utils.NewUUID()
	certdir := osenv.JujuXDGDataHomePath("x509")
	if certdir != "" {
		err := os.MkdirAll(certdir, 0700)
		if err == nil {
			pem, key, err := cert.NewCA("client", uuid.String(), expiry, 2048)
			if err != nil {
				return errors.Annotate(err, "failed to create new certificate")
			}
			public, err := cert.ComputePublicKey(pem)
			if err != nil {
				return errors.Annotate(err, "failed to compute x509 public key")
			}
			return cert.StoreDefaultX509Cert(certdir, pem, key, public)
		}
	}
	return nil
}
