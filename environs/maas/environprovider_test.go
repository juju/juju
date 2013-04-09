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
		"maas-server":     "http://maas.example.com/maas/api/1.0/",
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

// create a temporary file with the given content.  The caller is responsible
// for cleaning up the file.
func createTempFile(c *C, content []byte) string {
	file, err := ioutil.TempFile("", "")
	c.Assert(err, IsNil)
	filename := file.Name()
	err = ioutil.WriteFile(filename, content, 0644)
	c.Assert(err, IsNil)
	return filename
}

// InstanceId returns the instanceId of the machine read from the file
// _MAASInstanceFilename.
func (suite *EnvironProviderSuite) TestInstanceIdReadsInstanceIdFromMachineFile(c *C) {
	instanceId := "instance-id"
	info := machineInfo{instanceId, "hostname"}
	yaml, err := info.serializeYAML()
	c.Assert(err, IsNil)
	// Create a temporary file to act as the file where the instanceID
	// is stored.
	filename := createTempFile(c, yaml)
	defer os.Remove(filename)
	// "Monkey patch" the value of _MAASInstanceFilename with the path
	// to the temporary file.
	old_MAASInstanceFilename := _MAASInstanceFilename
	_MAASInstanceFilename = filename
	defer func() { _MAASInstanceFilename = old_MAASInstanceFilename }()

	provider := suite.environ.Provider()
	returnedInstanceId, err := provider.InstanceId()
	c.Assert(err, IsNil)
	c.Check(returnedInstanceId, Equals, state.InstanceId(instanceId))
}

// PublicAddress and PrivateAddress return the hostname of the machine read
// from the file _MAASInstanceFilename.
func (suite *EnvironProviderSuite) TestPrivatePublicAddressReadsHostnameFromMachineFile(c *C) {
	hostname := "myhostname"
	info := machineInfo{"instance-id", hostname}
	yaml, err := info.serializeYAML()
	c.Assert(err, IsNil)
	// Create a temporary file to act as the file where the instanceID
	// is stored.
	filename := createTempFile(c, yaml)
	defer os.Remove(filename)
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
