// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"crypto/x509"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/version"
)

var certDir = filepath.FromSlash(paths.MustSucceed(paths.CertDir(version.Current.Series)))

// CreateCertPool creates a new x509.CertPool and adds in the caCert passed
// in.  All certs from the cert directory (/etc/juju/cert.d on ubuntu) are
// also added.
func CreateCertPool(caCert string) (*x509.CertPool, error) {

	pool := x509.NewCertPool()
	if caCert != "" {
		xcert, err := cert.ParseCert(caCert)
		if err != nil {
			return nil, errors.Trace(err)
		}
		pool.AddCert(xcert)
	}

	count := processCertDir(pool)
	if count >= 0 {
		logger.Debugf("added %d certs to the pool from %s", count, certDir)
	}

	return pool, nil
}

// processCertDir iterates through the certDir looking for *.pem files.
// Each pem file is read in turn and added to the pool.  A count of the number
// of successful certificates processed is returned.
func processCertDir(pool *x509.CertPool) (count int) {
	fileInfo, err := os.Stat(certDir)
	if os.IsNotExist(err) {
		logger.Tracef("cert dir %q does not exist", certDir)
		return -1
	}
	if err != nil {
		logger.Infof("unexpected error reading cert dir: %s", err)
		return -1
	}
	if !fileInfo.IsDir() {
		logger.Infof("cert dir %q is not a directory", certDir)
		return -1
	}

	matches, err := filepath.Glob(filepath.Join(certDir, "*.pem"))
	if err != nil {
		logger.Infof("globbing files failed: %s", err)
		return -1
	}

	for _, match := range matches {
		data, err := ioutil.ReadFile(match)
		if err != nil {
			logger.Infof("error reading %q: %v", match, err)
			continue
		}
		certificate, err := cert.ParseCert(string(data))
		if err != nil {
			logger.Infof("error parsing cert %q: %v", match, err)
			continue
		}
		pool.AddCert(certificate)
		count++
	}
	return count
}
