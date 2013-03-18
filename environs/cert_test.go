package environs_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/testing"
)

type EnvironsCertSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&EnvironsCertSuite{})

func (*EnvironsCertSuite) TestWriteCertAndKeyToHome(c *C) {
	defer testing.MakeFakeHome(c, testing.PeckhamConfig).Restore()

	cert := []byte("a cert")
	key := []byte("a key")
	err := environs.WriteCertAndKeyToHome("foo", cert, key)
	c.Assert(err, IsNil)

	// Check that the generated CA key has been written correctly.
	caCertPEM, err := ioutil.ReadFile(testing.HomePath(".juju", "foo-cert.pem"))
	c.Assert(err, IsNil)
	c.Assert(string(caCertPEM), Equals, "a cert")

	caKeyPEM, err := ioutil.ReadFile(testing.HomePath(".juju", "foo-private-key.pem"))
	c.Assert(err, IsNil)
	c.Assert(string(caKeyPEM), Equals, "a key")
}

func (*EnvironsCertSuite) TestEnsureCertificateMissingKey(c *C) {
	defer testing.MakeFakeHome(c, testing.SingleEnvConfig).Restore()
	envName := testing.SampleEnvName

	keyPath := testing.HomePath(".juju", envName+"-cert.pem")
	ioutil.WriteFile(keyPath, []byte(testing.CACert), 0600)

	// Need to create the environment after the cert has been written.
	env, err := environs.NewFromName(envName)
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
	env, err := environs.NewFromName(testing.SampleEnvName)
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
	env, err := environs.NewFromName(testing.SampleEnvName)
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
