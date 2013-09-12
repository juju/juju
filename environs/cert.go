// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/environs/config"
)

type CreatedCert bool

const (
	CertCreated CreatedCert = true
	CertExists  CreatedCert = false
)

// WriteCertAndKey writes the provided certificate and key
// to the juju home directory, creating it if necessary,
func WriteCertAndKey(name string, cert, key []byte) error {
	// If the juju home directory doesn't exist, create it.
	jujuHome := config.JujuHome()
	if err := os.MkdirAll(jujuHome, 0775); err != nil {
		return err
	}
	path := filepath.Join(jujuHome, name)
	if err := ioutil.WriteFile(path+"-cert.pem", cert, 0644); err != nil {
		return err
	}
	return ioutil.WriteFile(path+"-private-key.pem", key, 0600)
}

func generateCertificate(environ Environ, writeCertAndKey func(environName string, cert, key []byte) error) error {
	cfg := environ.Config()
	caCert, caKey, err := cert.NewCA(environ.Name(), time.Now().UTC().AddDate(10, 0, 0))
	if err != nil {
		return err
	}
	m := cfg.AllAttrs()
	m["ca-cert"] = string(caCert)
	m["ca-private-key"] = string(caKey)
	cfg, err = config.New(config.NoDefaults, m)
	if err != nil {
		return fmt.Errorf("cannot create environment configuration with new CA: %v", err)
	}
	if err := environ.SetConfig(cfg); err != nil {
		return fmt.Errorf("cannot set environment configuration with CA: %v", err)
	}
	if err := writeCertAndKey(environ.Name(), caCert, caKey); err != nil {
		return fmt.Errorf("cannot write CA certificate and key: %v", err)
	}
	return nil
}

// EnsureCertificate makes sure that there is a certificate and private key
// for the specified environment.  If one does not exist, then a certificate
// is generated.
func EnsureCertificate(environ Environ, writeCertAndKey func(environName string, cert, key []byte) error) (CreatedCert, error) {
	cfg := environ.Config()
	_, hasCACert := cfg.CACert()
	_, hasCAKey := cfg.CAPrivateKey()

	if hasCACert && hasCAKey {
		// All is good in the world.
		return CertExists, nil
	}
	// It is not possible to create an environment that has a private key, but no certificate.
	if hasCACert && !hasCAKey {
		return CertExists, fmt.Errorf("environment configuration with a certificate but no CA private key")
	}

	return CertCreated, generateCertificate(environ, writeCertAndKey)
}
