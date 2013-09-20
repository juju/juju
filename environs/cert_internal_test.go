// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/testing"
)

// EnvironsCertSuite tests the internal functions defined in environs/cert.go
type EnvironsCertSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&EnvironsCertSuite{})

type testCerts struct {
	cert []byte
	key  []byte
}

func (*EnvironsCertSuite) TestGenerateCertificate(c *gc.C) {
	defer testing.MakeSampleHome(c).Restore()
	env, err := PrepareFromName(testing.SampleEnvName)
	c.Assert(err, gc.IsNil)

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
	c.Assert(cfgCertOK, gc.Equals, true)
	c.Assert(cfgKeyOK, gc.Equals, true)
	c.Assert(cfgCertPEM, gc.DeepEquals, savedCerts.cert)
	c.Assert(cfgKeyPEM, gc.DeepEquals, savedCerts.key)

	// Check the common name of the generated cert
	caCert, _, err := cert.ParseCertAndKey(cfgCertPEM, cfgKeyPEM)
	c.Assert(err, gc.IsNil)
	c.Assert(caCert.Subject.CommonName, gc.Equals, `juju-generated CA for environment "`+testing.SampleEnvName+`"`)
}
