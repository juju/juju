// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"crypto/x509"

	"github.com/juju/errors"
	"github.com/lxc/lxd"
)

type rawClientServerMethods interface {
	WaitForSuccess(waitURL string) error
	SetServerConfig(key string, value string) (*lxd.Response, error)
	CertificateAdd(cert *x509.Certificate, name string) error
}

type clientServerMethods struct {
	raw rawServerMethods
}

func (c clientServerMethods) setUpRemote(cert *x509.Certificate, name string) error {
	resp, err := c.raw.SetServerConfig("core.https_address", "[::]")
	if err != nil {
		return errors.Trace(err)
	}
	if err := c.raw.WaitForSuccess(resp.Operation); err != nil {
		// TODO(ericsnow) Handle different failures (from the async
		// operation) differently?
		return errors.Trace(err)
	}

	if err := c.raw.CertificateAdd(cert, name); err != nil {
		return errors.Trace(err)
	}

	return nil
}
