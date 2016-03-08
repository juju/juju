// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type BaseSuite struct {
	testing.IsolationSuite

	Cert   *Cert
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Cert = &Cert{
		Name:    "some cert",
		CertPEM: []byte("<a valid PEM-encoded x.509 cert>"),
		KeyPEM:  []byte("<a valid PEM-encoded x.509 key>"),
	}
}

