// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/loggo"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	utilscert "github.com/juju/utils/cert"
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
	loggo.DefaultContext().ResetLoggerLevels()
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

		_, _, err = utilscert.ParseCertAndKey(certPEM, keyPEM)
		c.Check(err, jc.ErrorIsNil)

		err = cert.Verify(certPEM, testing.CACert, time.Now())
		c.Assert(err, jc.ErrorIsNil)
		err = cert.Verify(certPEM, testing.CACert, time.Now().AddDate(9, 0, 0))
		c.Assert(err, jc.ErrorIsNil)
		err = cert.Verify(certPEM, testing.CACert, time.Now().AddDate(10, 0, 1))
		c.Assert(err, gc.NotNil)
		srvCert, err := utilscert.ParseCert(certPEM)
		c.Assert(err, jc.ErrorIsNil)
		sanIPs := make([]string, len(srvCert.IPAddresses))
		for i, ip := range srvCert.IPAddresses {
			sanIPs[i] = ip.String()
		}
		c.Assert(sanIPs, jc.SameContents, test.sanValues)
	}
}

var validateTests = []struct {
	about       string
	config      controller.Config
	expectError string
}{{
	about:       "missing CA cert",
	expectError: `missing CA certificate`,
}, {
	about: "bad CA cert",
	config: controller.Config{
		controller.CACertKey: "xxx",
	},
	expectError: `bad CA certificate in configuration: no certificates found`,
}, {
	about: "bad controller UUID",
	config: controller.Config{
		controller.ControllerUUIDKey: "xxx",
		controller.CACertKey:         testing.CACert,
	},
	expectError: `controller-uuid: expected UUID, got string\("xxx"\)`,
}, {
	about: "HTTPS identity URL OK",
	config: controller.Config{
		controller.IdentityURL: "https://0.1.2.3/foo",
		controller.CACertKey:   testing.CACert,
	},
}, {
	about: "HTTP identity URL requires public key",
	config: controller.Config{
		controller.IdentityURL: "http://0.1.2.3/foo",
		controller.CACertKey:   testing.CACert,
	},
	expectError: `URL needs to be https when identity-public-key not provided`,
}, {
	about: "HTTP identity URL OK if public key is provided",
	config: controller.Config{
		controller.IdentityPublicKey: `o/yOqSNWncMo1GURWuez/dGR30TscmmuIxgjztpoHEY=`,
		controller.IdentityURL:       "http://0.1.2.3/foo",
		controller.CACertKey:         testing.CACert,
	},
}, {
	about: "invalid identity public key",
	config: controller.Config{
		controller.IdentityPublicKey: `xxxx`,
		controller.CACertKey:         testing.CACert,
	},
	expectError: `invalid identity public key: wrong length for base64 key, got 3 want 32`,
}}

func (s *ConfigSuite) TestValidate(c *gc.C) {
	for i, test := range validateTests {
		c.Logf("test %d: %v", i, test.about)
		err := test.config.Validate()
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
	}
}
