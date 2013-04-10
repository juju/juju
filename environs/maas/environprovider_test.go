package maas

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"os"
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
		"maas-server":     "http://maas.example.com/maas/",
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

func (suite *EnvironProviderSuite) TestInstanceIdReadsInstanceIdFile(c *C) {
	instanceId := "instance-id"
	// Create a temporary file to act as the file where the instanceID
	// is stored.
	file, err := ioutil.TempFile("", "")
	c.Assert(err, IsNil)
	filename := file.Name()
	defer os.Remove(filename)
	err = ioutil.WriteFile(filename, []byte(instanceId), 0644)
	c.Assert(err, IsNil)
	// "Monkey patch" the value of _MAASInstanceIDFilename with the path
	// to the temporary file.
	old_MAASInstanceIDFilename := _MAASInstanceIDFilename
	_MAASInstanceIDFilename = filename
	defer func() { _MAASInstanceIDFilename = old_MAASInstanceIDFilename }()

	provider := suite.environ.Provider()
	returnedInstanceId, err := provider.InstanceId()
	c.Assert(err, IsNil)
	c.Check(returnedInstanceId, Equals, state.InstanceId(instanceId))
}
