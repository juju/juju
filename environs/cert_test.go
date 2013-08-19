// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"io/ioutil"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
)

type EnvironsCertSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&EnvironsCertSuite{})

func (*EnvironsCertSuite) TestWriteCertAndKey(c *C) {
	defer testing.MakeEmptyFakeHome(c).Restore()

	// Ensure that the juju home path is different
	// from $HOME/.juju to check that WriteCertAndKey
	// isn't just using $HOME.
	config.SetJujuHome(c.MkDir())

	cert, key := []byte("a cert"), []byte("a key")
	err := environs.WriteCertAndKey("foo", cert, key)
	c.Assert(err, IsNil)

	// Check that the generated CA key has been written correctly.
	caCertPEM, err := ioutil.ReadFile(config.JujuHomePath("foo-cert.pem"))
	c.Assert(err, IsNil)
	c.Assert(caCertPEM, DeepEquals, cert)

	caKeyPEM, err := ioutil.ReadFile(config.JujuHomePath("foo-private-key.pem"))
	c.Assert(err, IsNil)
	c.Assert(caKeyPEM, DeepEquals, key)

}

func (*EnvironsCertSuite) TestEnsureCertificateMissingKey(c *C) {
	defer testing.MakeFakeHome(c, testing.SingleEnvConfig).Restore()
	envName := testing.SampleEnvName

	keyPath := testing.HomePath(".juju", envName+"-cert.pem")
	ioutil.WriteFile(keyPath, []byte(testing.CACert), 0600)

	// Need to create the environment after the cert has been written.
	env, err := environs.PrepareFromName(envName)
	c.Assert(err, IsNil)

	writeCalled := false
	_, err = environs.EnsureCertificate(env, func(name string, cert, key []byte) error {
		writeCalled = true
		return nil
	})
	c.Assert(err, ErrorMatches, "environment configuration with a certificate but no CA private key")
	c.Assert(writeCalled, Equals, false)
}

func (*EnvironsCertSuite) TestEnsureCertificateExisting(c *C) {
	defer testing.MakeSampleHome(c).Restore()
	env, err := environs.PrepareFromName(testing.SampleEnvName)
	c.Assert(err, IsNil)
	writeCalled := false
	created, err := environs.EnsureCertificate(env, func(name string, cert, key []byte) error {
		writeCalled = true
		return nil
	})
	c.Assert(err, IsNil)
	c.Assert(created, Equals, environs.CertExists)
	c.Assert(writeCalled, Equals, false)
}

func (*EnvironsCertSuite) TestEnsureCertificate(c *C) {
	defer testing.MakeFakeHome(c, testing.SingleEnvConfig).Restore()
	env, err := environs.PrepareFromName(testing.SampleEnvName)
	c.Assert(err, IsNil)
	writeCalled := false
	created, err := environs.EnsureCertificate(env, func(name string, cert, key []byte) error {
		writeCalled = true
		return nil
	})
	c.Assert(err, IsNil)
	c.Assert(created, Equals, environs.CertCreated)
	c.Assert(writeCalled, Equals, true)
}
