package environs_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/testing"
)

type EnvironsCertBlackBoxSuite struct {
}

var _ = Suite(&EnvironsCertBlackBoxSuite{})

func (*EnvironsCertBlackBoxSuite) TestEnsureCertificateMissingKey(c *C) {
	defer testing.MakeFakeHome(c, testing.PeckhamConfig).Restore()
	envName := "peckham"
	keyPath := testing.HomePath(".juju", envName+"-cert.pem")
	ioutil.WriteFile(keyPath, []byte(testing.CACert), 0600)

	env, err := environs.NewFromName(envName)
	c.Assert(err, IsNil)
	_, err = environs.EnsureCertificate(env)
	c.Assert(err, ErrorMatches, "environment configuration with a certificate but no CA private key")
}

func (*EnvironsCertBlackBoxSuite) TestEnsureCertificateExisting(c *C) {
	defer testing.MakeFakeHome(c, testing.PeckhamConfig, "peckham").Restore()
	envName := "peckham"
	env, err := environs.NewFromName(envName)
	c.Assert(err, IsNil)
	created, err := environs.EnsureCertificate(env)
	c.Assert(err, IsNil)
	c.Assert(created, Equals, environs.CertExists)
}

func (*EnvironsCertBlackBoxSuite) TestEnsureCertificate(c *C) {
	defer testing.MakeFakeHome(c, testing.PeckhamConfig).Restore()
	envName := "peckham"
	env, err := environs.NewFromName(envName)
	c.Assert(err, IsNil)
	created, err := environs.EnsureCertificate(env)
	c.Assert(err, IsNil)
	c.Assert(created, Equals, environs.CertCreated)
	// Certificate attributes tested in cert_test.go
}
