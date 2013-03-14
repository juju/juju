package main

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type BootstrapSuite struct {
	testing.LoggingSuite
	testing.MgoSuite
}

var _ = Suite(&BootstrapSuite{})

func (s *BootstrapSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *BootstrapSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
}

func (s *BootstrapSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *BootstrapSuite) TearDownTest(c *C) {
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
	dummy.Reset()
}

func (*BootstrapSuite) TestWriteCertAndKeyToHome(c *C) {
	defer testing.MakeFakeHome(c, envConfig).Restore()

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

func (*BootstrapSuite) TestCheckCertificateMissingKey(c *C) {
	defer testing.MakeFakeHome(c, envConfig).Restore()
	envName := "peckham"
	keyPath := filepath.Join(os.Getenv("HOME"), ".juju", envName+"-cert.pem")
	ioutil.WriteFile(keyPath, []byte(testing.CACert), 0600)

	env, err := environs.NewFromName(envName)
	c.Assert(err, IsNil)
	err = checkCertificate(env)

	c.Assert(err, ErrorMatches, "environment configuration with a certificate but no CA private key")
}

func (*BootstrapSuite) TestRunGeneratesCertificate(c *C) {
	defer testing.MakeFakeHome(c, envConfig).Restore()
	envName := "peckham"
	_, err := testing.RunCommand(c, new(BootstrapCommand), nil)
	c.Assert(err, IsNil)

	// Check that the CA certificate and key have been automatically generated
	// for the environment.
	info, err := os.Stat(testing.HomePath(".juju", envName+"-cert.pem"))
	c.Assert(err, IsNil)
	c.Assert(info.Size() > 0, Equals, true)
	info, err = os.Stat(testing.HomePath(".juju", envName+"-private-key.pem"))
	c.Assert(err, IsNil)
	c.Assert(info.Size() > 0, Equals, true)

	// Check that the environment validates the cert and key.
	env, err := environs.NewFromName(envName)
	c.Assert(err, IsNil)

	cfgCertPEM, cfgCertOK := env.Config().CACert()
	cfgKeyPEM, cfgKeyOK := env.Config().CAPrivateKey()
	c.Assert(cfgCertOK, Equals, true)
	c.Assert(cfgKeyOK, Equals, true)

	// Check the common name of the generated cert
	caCert, _, err := cert.ParseCertAndKey(cfgCertPEM, cfgKeyPEM)
	c.Assert(err, IsNil)
	c.Assert(caCert.Subject.CommonName, Equals, `juju-generated CA for environment peckham`)
}

func (*BootstrapSuite) TestBootstrapCommandNoParams(c *C) {
	defer testing.MakeFakeHome(c, envConfig).Restore()
	opc, errc := runCommand(new(BootstrapCommand))
	c.Check(<-errc, IsNil)
	c.Check((<-opc).(dummy.OpBootstrap).Env, Equals, "peckham")
}

func (*BootstrapSuite) TestBootstrapCommandUploadTools(c *C) {
	defer testing.MakeFakeHome(c, envConfig).Restore()
	// bootstrap with tool uploading - checking that a file
	// is uploaded should be sufficient, as the detailed semantics
	// of UploadTools are tested in environs.
	opc, errc := runCommand(new(BootstrapCommand), "--upload-tools")
	c.Check(<-errc, IsNil)
	c.Check((<-opc).(dummy.OpPutFile).Env, Equals, "peckham")
	c.Check((<-opc).(dummy.OpBootstrap).Env, Equals, "peckham")

	envs, err := environs.ReadEnvirons("")
	c.Assert(err, IsNil)
	env, err := envs.Open("peckham")
	c.Assert(err, IsNil)

	tools, err := environs.FindTools(env, version.Current, environs.CompatVersion)
	c.Assert(err, IsNil)
	resp, err := http.Get(tools.URL)
	c.Assert(err, IsNil)
	defer resp.Body.Close()

	err = agent.UnpackTools(c.MkDir(), tools, resp.Body)
	c.Assert(err, IsNil)
}

func (*BootstrapSuite) TestBootstrapCommandBrokenEnvironment(c *C) {
	defer testing.MakeFakeHome(c, envConfig).Restore()
	opc, errc := runCommand(new(BootstrapCommand), "-e", "brokenenv")
	c.Check(<-errc, ErrorMatches, "dummy.Bootstrap is broken")
	c.Check(<-opc, IsNil)
}

func (*BootstrapSuite) TestMissingEnvironment(c *C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "empty").Restore()
	// bootstrap without an environments.yaml
	ctx := testing.Context(c)
	code := cmd.Main(&BootstrapCommand{}, ctx, nil)
	c.Check(code, Equals, 1)
	errStr := ctx.Stderr.(*bytes.Buffer).String()
	strippedErr := strings.Replace(errStr, "\n", "", -1)
	c.Assert(strippedErr, Matches, ".*No juju environment configuration file exists.*")
}
