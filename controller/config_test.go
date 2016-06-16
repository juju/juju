// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/loggo"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cert"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type ConfigSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	home string
}

var _ = gc.Suite(&ConfigSuite{})

func (s *ConfigSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	// Make sure that the defaults are used, which
	// is <root>=WARNING
	loggo.ResetLoggers()
}

func (s *ConfigSuite) TestGenerateControllerCertAndKey(c *gc.C) {
	// Add a cert.
	s.FakeHomeSuite.Home.AddFiles(c, gitjujutesting.TestFile{".ssh/id_rsa.pub", "rsa\n"})

	for _, test := range []struct {
		caCert    string
		caKey     string
		sanValues []string
	}{{
		caCert: testing.CACert,
		caKey:  testing.CAKey,
	}, {
		caCert:    testing.CACert,
		caKey:     testing.CAKey,
		sanValues: []string{"10.0.0.1", "192.168.1.1"},
	}} {
		certPEM, keyPEM, err := controller.GenerateControllerCertAndKey(test.caCert, test.caKey, test.sanValues)
		c.Assert(err, jc.ErrorIsNil)

		_, _, err = cert.ParseCertAndKey(certPEM, keyPEM)
		c.Check(err, jc.ErrorIsNil)

		err = cert.Verify(certPEM, testing.CACert, time.Now())
		c.Assert(err, jc.ErrorIsNil)
		err = cert.Verify(certPEM, testing.CACert, time.Now().AddDate(9, 0, 0))
		c.Assert(err, jc.ErrorIsNil)
		err = cert.Verify(certPEM, testing.CACert, time.Now().AddDate(10, 0, 1))
		c.Assert(err, gc.NotNil)
		srvCert, err := cert.ParseCert(certPEM)
		c.Assert(err, jc.ErrorIsNil)
		sanIPs := make([]string, len(srvCert.IPAddresses))
		for i, ip := range srvCert.IPAddresses {
			sanIPs[i] = ip.String()
		}
		c.Assert(sanIPs, jc.SameContents, test.sanValues)
	}
}
