// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs/config"
)

type EnvironProviderSuite struct {
	ProviderSuite
}

var _ = Suite(new(EnvironProviderSuite))

func (suite *EnvironProviderSuite) TestSecretAttrsReturnsSensitiveMAASAttributes(c *C) {
	testJujuHome := c.MkDir()
	defer config.SetJujuHome(config.SetJujuHome(testJujuHome))
	const oauth = "aa:bb:cc"
	attrs := map[string]interface{}{
		"maas-oauth":      oauth,
		"maas-server":     "http://maas.testing.invalid/maas/",
		"name":            "wheee",
		"type":            "maas",
		"authorized-keys": "I-am-not-a-real-key",
	}
	config, err := config.New(attrs)
	c.Assert(err, IsNil)

	secretAttrs, err := suite.environ.Provider().SecretAttrs(config)
	c.Assert(err, IsNil)

	expectedAttrs := map[string]interface{}{"maas-oauth": oauth}
	c.Check(secretAttrs, DeepEquals, expectedAttrs)
}

// create a temporary file with the given content.  The file will be cleaned
// up at the end of the test calling this method.
func createTempFile(c *C, content []byte) string {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, IsNil)
	filename := file.Name()
	err = ioutil.WriteFile(filename, content, 0644)
	c.Assert(err, IsNil)
	return filename
}

// PublicAddress and PrivateAddress return the hostname of the machine read
// from the file _MAASInstanceFilename.
func (suite *EnvironProviderSuite) TestPrivatePublicAddressReadsHostnameFromMachineFile(c *C) {
	hostname := "myhostname"
	info := machineInfo{hostname}
	yaml, err := goyaml.Marshal(info)
	c.Assert(err, IsNil)
	// Create a temporary file to act as the file where the instanceID
	// is stored.
	filename := createTempFile(c, yaml)
	// "Monkey patch" the value of _MAASInstanceFilename with the path
	// to the temporary file.
	old_MAASInstanceFilename := _MAASInstanceFilename
	_MAASInstanceFilename = filename
	defer func() { _MAASInstanceFilename = old_MAASInstanceFilename }()

	provider := suite.environ.Provider()
	publicAddress, err := provider.PublicAddress()
	c.Assert(err, IsNil)
	c.Check(publicAddress, Equals, hostname)
	privateAddress, err := provider.PrivateAddress()
	c.Assert(err, IsNil)
	c.Check(privateAddress, Equals, hostname)
}

func (suite *EnvironProviderSuite) TestOpenReturnsNilInterfaceUponFailure(c *C) {
	testJujuHome := c.MkDir()
	defer config.SetJujuHome(config.SetJujuHome(testJujuHome))
	const oauth = "wrongly-formatted-oauth-string"
	attrs := map[string]interface{}{
		"maas-oauth":      oauth,
		"maas-server":     "http://maas.testing.invalid/maas/",
		"name":            "wheee",
		"type":            "maas",
		"authorized-keys": "I-am-not-a-real-key",
	}
	config, err := config.New(attrs)
	c.Assert(err, IsNil)
	env, err := suite.environ.Provider().Open(config)
	// When Open() fails (i.e. returns a non-nil error), it returns an
	// environs.Environ interface object with a nil value and a nil
	// type.
	c.Check(env, Equals, nil)
	c.Check(err, ErrorMatches, ".*malformed maas-oauth.*")
}
