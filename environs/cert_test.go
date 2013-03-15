package environs

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/testing"
)

// EnvironsCertSuite tests the internal functions defined in environs/cert.go
type EnvironsCertSuite struct {
}

var _ = Suite(&EnvironsCertSuite{})

func (*EnvironsCertSuite) TestWriteCertAndKeyToHome(c *C) {
	defer testing.MakeFakeHome(c, testing.PeckhamConfig).Restore()

	cert := []byte("a cert")
	key := []byte("a key")
	err := writeCertAndKeyToHome("foo", cert, key)
	c.Assert(err, IsNil)

	// Check that the generated CA key has been written correctly.
	caCertPEM, err := ioutil.ReadFile(testing.HomePath(".juju", "foo-cert.pem"))
	c.Assert(err, IsNil)
	c.Assert(string(caCertPEM), Equals, "a cert")

	caKeyPEM, err := ioutil.ReadFile(testing.HomePath(".juju", "foo-private-key.pem"))
	c.Assert(err, IsNil)
	c.Assert(string(caKeyPEM), Equals, "a key")
}

func (*EnvironsCertSuite) TestGenerateCertificate(c *C) {
	defer testing.MakeFakeHome(c, testing.PeckhamConfig).Restore()
	envName := "peckham"
	env, err := NewFromName(envName)
	c.Assert(err, IsNil)

	generateCertificate(env)

	// Check that the generated CA key has been written correctly.
	caCertPEM, err := ioutil.ReadFile(testing.HomePath(".juju", envName+"-cert.pem"))
	c.Assert(err, IsNil)
	caKeyPEM, err := ioutil.ReadFile(testing.HomePath(".juju", envName+"-private-key.pem"))
	c.Assert(err, IsNil)

	// Check that the cert and key have been set correctly in the configuration
	cfgCertPEM, cfgCertOK := env.Config().CACert()
	cfgKeyPEM, cfgKeyOK := env.Config().CAPrivateKey()
	c.Assert(cfgCertOK, Equals, true)
	c.Assert(cfgKeyOK, Equals, true)
	c.Assert(cfgCertPEM, DeepEquals, caCertPEM)
	c.Assert(cfgKeyPEM, DeepEquals, caKeyPEM)

	// Check the common name of the generated cert
	caCert, _, err := cert.ParseCertAndKey(cfgCertPEM, cfgKeyPEM)
	c.Assert(err, IsNil)
	c.Assert(caCert.Subject.CommonName, Equals, `juju-generated CA for environment peckham`)
}
