// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/testing"
)

// EnvironsCertSuite tests the internal functions defined in environs/cert.go
type EnvironsCertSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&EnvironsCertSuite{})

type testCerts struct {
	cert []byte
	key  []byte
}

func (*EnvironsCertSuite) TestGenerateCertificate(c *C) {
	defer testing.MakeSampleHome(c).Restore()
	env, err := NewFromName(testing.SampleEnvName)
	c.Assert(err, IsNil)

	var savedCerts testCerts
	writeFunc := func(name string, cert, key []byte) error {
		savedCerts.cert = cert
		savedCerts.key = key
		return nil
	}
	generateCertificate(env, writeFunc)

	// Check that the cert and key have been set correctly in the configuration
	cfgCertPEM, cfgCertOK := env.Config().CACert()
	cfgKeyPEM, cfgKeyOK := env.Config().CAPrivateKey()
	c.Assert(cfgCertOK, Equals, true)
	c.Assert(cfgKeyOK, Equals, true)
	c.Assert(cfgCertPEM, DeepEquals, savedCerts.cert)
	c.Assert(cfgKeyPEM, DeepEquals, savedCerts.key)

	// Check the common name of the generated cert
	caCert, _, err := cert.ParseCertAndKey(cfgCertPEM, cfgKeyPEM)
	c.Assert(err, IsNil)
	c.Assert(caCert.Subject.CommonName, Equals, `juju-generated CA for environment "`+testing.SampleEnvName+`"`)
}
