package environs_test

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
	"os"
	"path/filepath"
	"time"
)

type bootstrapSuite struct {
	oldHome string
	testing.LoggingSuite
}

var _ = Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.oldHome = os.Getenv("HOME")
	home := c.MkDir()
	os.Setenv("HOME", home)
	err := os.Mkdir(filepath.Join(home, ".juju"), 0777)
	c.Assert(err, IsNil)
}

func (s *bootstrapSuite) TearDownTest(c *C) {
	os.Setenv("HOME", s.oldHome)
}

func (s *bootstrapSuite) TestBootstrapKeyGeneration(c *C) {
	env := newEnviron("foo", nil, nil)
	err := environs.Bootstrap(env, false, nil)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	_, _, err = cert.ParseCertAndKey(env.certPEM, env.keyPEM)
	c.Assert(err, IsNil)

	// Check that the generated CA key has been written correctly.
	caCertPEM, err := ioutil.ReadFile(filepath.Join(os.Getenv("HOME"), ".juju", "foo-cert.pem"))
	c.Assert(err, IsNil)
	caKeyPEM, err := ioutil.ReadFile(filepath.Join(os.Getenv("HOME"), ".juju", "foo-private-key.pem"))
	c.Assert(err, IsNil)

	// Check that the cert and key have been set correctly in the configuration
	cfgCertPEM, cfgCertOK := env.cfg.CACert()
	cfgKeyPEM, cfgKeyOK := env.cfg.CAPrivateKey()
	c.Assert(cfgCertOK, Equals, true)
	c.Assert(cfgKeyOK, Equals, true)
	c.Assert(cfgCertPEM, DeepEquals, caCertPEM)
	c.Assert(cfgKeyPEM, DeepEquals, caKeyPEM)

	caCert, _, err := cert.ParseCertAndKey(cfgCertPEM, cfgKeyPEM)
	c.Assert(err, IsNil)
	c.Assert(caCert.Subject.CommonName, Equals, `juju-generated CA for environment foo`)

	verifyCert(c, env.certPEM, caCertPEM)
}

func verifyCert(c *C, srvCertPEM, caCertPEM []byte) {
	err := cert.Verify(srvCertPEM, caCertPEM, time.Now())
	c.Assert(err, IsNil)
	err = cert.Verify(srvCertPEM, caCertPEM, time.Now().AddDate(9, 0, 0))
	c.Assert(err, IsNil)
}

func (s *bootstrapSuite) TestBootstrapFuncKeyGeneration(c *C) {
	env := newEnviron("foo", nil, nil)
	var savedCert, savedKey []byte
	err := environs.Bootstrap(env, false, func(name string, cert, key []byte) error {
		savedCert = cert
		savedKey = key
		return nil
	})
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	_, _, err = cert.ParseCertAndKey(env.certPEM, env.keyPEM)
	c.Assert(err, IsNil)

	// Check that the cert and key have been set correctly in the configuration
	cfgCertPEM, cfgCertOK := env.cfg.CACert()
	cfgKeyPEM, cfgKeyOK := env.cfg.CAPrivateKey()
	c.Assert(cfgCertOK, Equals, true)
	c.Assert(cfgKeyOK, Equals, true)
	c.Assert(cfgCertPEM, DeepEquals, savedCert)
	c.Assert(cfgKeyPEM, DeepEquals, savedKey)

	caCert, _, err := cert.ParseCertAndKey(cfgCertPEM, cfgKeyPEM)
	c.Assert(err, IsNil)
	c.Assert(caCert.Subject.CommonName, Equals, `juju-generated CA for environment foo`)

	verifyCert(c, env.certPEM, cfgCertPEM)
}

func panicWrite(name string, cert, key []byte) error {
	panic("writeCertAndKey called unexpectedly")
}

func (s *bootstrapSuite) TestBootstrapExistingKey(c *C) {
	env := newEnviron("foo", []byte(testing.CACert), []byte(testing.CAKey))
	err := environs.Bootstrap(env, false)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)

	verifyCert(c, env.certPEM, []byte(testing.CACert))
}

func (s *bootstrapSuite) TestBootstrapUploadTools(c *C) {
	env := newEnviron("foo", nil, nil)
	err := environs.Bootstrap(env, false)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.uploadTools, Equals, false)

	env = newEnviron("foo", nil, nil)
	err = environs.Bootstrap(env, true)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.uploadTools, Equals, true)
}

type bootstrapEnviron struct {
	name             string
	cfg              *config.Config
	environs.Environ // stub out all methods we don't care about.

	// The following fields are filled in when Bootstrap is called.
	bootstrapCount int
	uploadTools    bool
	certPEM        []byte
	keyPEM         []byte
}

func newEnviron(name string, caCertPEM, caKeyPEM []byte) *bootstrapEnviron {
	m := map[string]interface{}{
		"name":            name,
		"type":            "test",
		"authorized-keys": "foo",
		"ca-cert":         "",
		"ca-private-key":  "",
	}
	if caCertPEM != nil {
		m["ca-cert"] = string(caCertPEM)
	}
	if caKeyPEM != nil {
		m["ca-private-key"] = string(caKeyPEM)
	}
	cfg, err := config.New(m)
	if err != nil {
		panic(fmt.Errorf("cannot create config from %#v: %v", m, err))
	}
	return &bootstrapEnviron{
		name: name,
		cfg:  cfg,
	}
}

func (e *bootstrapEnviron) Name() string {
	return e.name
}

func (e *bootstrapEnviron) Bootstrap(uploadTools bool, certPEM, keyPEM []byte) error {
	e.bootstrapCount++
	e.uploadTools = uploadTools
	e.certPEM = certPEM
	e.keyPEM = keyPEM
	return nil
}

func (e *bootstrapEnviron) Config() *config.Config {
	return e.cfg
}

func (e *bootstrapEnviron) SetConfig(cfg *config.Config) error {
	e.cfg = cfg
	return nil
}
