// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
)

type EnvironProviderSuite struct {
	ProviderSuite
}

var _ = Suite(new(EnvironProviderSuite))

func (EnvironProviderSuite) TestOpen(c *C) {
	prov := azureEnvironProvider{}
	attrs := makeAzureConfigMap(c)
	attrs["name"] = "my-shiny-new-env"
	cfg, err := config.New(attrs)
	c.Assert(err, IsNil)

	env, err := prov.Open(cfg)
	c.Assert(err, IsNil)

	c.Check(env.Name(), Equals, attrs["name"])
}

// create a temporary file with a valid WALinux config built using the given parameters.
// The file will be cleaned up at the end of the test calling this method.
func writeWALASharedConfig(c *C, deploymentId string, deploymentName string, internalAddress string) string {
	configTemplateXML := `
	<SharedConfig version="1.0.0.0" goalStateIncarnation="1">
	  <Deployment name="%s" guid="{495985a8-8e5a-49aa-826f-d1f7f51045b6}" incarnation="0">
	    <Service name="%s" guid="{00000000-0000-0000-0000-000000000000}" />
	    <ServiceInstance name="%s" guid="{9806cac7-e566-42b8-9ecb-de8da8f69893}" />
	  </Deployment>
	  <Instances>
            <Instance id="gwaclroleldc1o5p" address="%s">
            </Instance>
          </Instances>
        </SharedConfig>`
	config := fmt.Sprintf(configTemplateXML, deploymentId, deploymentName, deploymentId, internalAddress)
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, IsNil)
	filename := file.Name()
	err = ioutil.WriteFile(filename, []byte(config), 0644)
	c.Assert(err, IsNil)
	return filename
}

// overrideWALASharedConfig:
// - creates a temporary file with a valid WALinux config built using the
// given parameters.  The file will be cleaned up at the end of the test
// calling this method.
// - monkey patches the value of '_WALAConfigPath' (the path to the WALA
// configuration file) so that it contains the path to the temporary file. 
// overrideWALASharedConfig returns a cleanup method that the caller *must*
// call in order to restore the original value of '_WALAConfigPath'
func overrideWALASharedConfig(c *C, deploymentId, deploymentName, internalAddress string) func() {
	filename := writeWALASharedConfig(c, deploymentId, deploymentName,
		internalAddress)
	oldConfigPath := _WALAConfigPath
	_WALAConfigPath = filename
	// return cleanup method to restore the original value of
	// '_WALAConfigPath'.
	return func() {
		_WALAConfigPath = oldConfigPath
	}
}

func (EnvironProviderSuite) TestParseWALASharedConfig(c *C) {
	deploymentId := "b6de4c4c7d4a49c39270e0c57481fd9b"
	deploymentName := "gwaclmachineex95rsek"
	internalAddress := "10.76.200.59"

	cleanup := overrideWALASharedConfig(c, deploymentId, deploymentName, internalAddress)
	defer cleanup()

	config, err := parseWALAConfig()
	c.Assert(err, IsNil)
	c.Check(config.Deployment.Name, Equals, deploymentId)
	c.Check(config.Deployment.Service.Name, Equals, deploymentName)
	c.Check(config.Instances[0].Address, Equals, internalAddress)
}

func (EnvironProviderSuite) TestConfigGetDeploymentName(c *C) {
	deploymentId := "b6de4c4c7d4a49c39270e0c57481fd9b"
	config := WALASharedConfig{Deployment: WALADeployment{Name: deploymentId, Service: WALADeploymentService{Name: "name"}}}

	c.Check(config.getDeploymentFQDN(), Equals, deploymentId+".cloudapp.net")
}

func (EnvironProviderSuite) TestConfigGetDeploymentHostname(c *C) {
	deploymentName := "gwaclmachineex95rsek"
	config := WALASharedConfig{Deployment: WALADeployment{Name: "id", Service: WALADeploymentService{Name: deploymentName}}}

	c.Check(config.getDeploymentName(), Equals, deploymentName)
}

func (EnvironProviderSuite) TestConfigGetInternalIP(c *C) {
	internalAddress := "10.76.200.59"
	config := WALASharedConfig{Instances: []WALAInstance{WALAInstance{Address: internalAddress}}}

	c.Check(config.getInternalIP(), Equals, internalAddress)
}

func (EnvironProviderSuite) TestPublicAddress(c *C) {
	deploymentId := "b6de4c4c7d4a49c39270e0c57481fd9b"
	cleanup := overrideWALASharedConfig(c, deploymentId, "name", "10.76.200.59")
	defer cleanup()

	expectedAddress := deploymentId + ".cloudapp.net"
	prov := azureEnvironProvider{}
	pubAddress, err := prov.PublicAddress()
	c.Assert(err, IsNil)
	c.Check(pubAddress, Equals, expectedAddress)
}

// azureEnvironProvider.PrivateAddress() currently returns the public address
// of the instance.  We need to figure out how to do instance-to-instance
// communication using the private IPs before we can use the Azure private
// address.
func (EnvironProviderSuite) TestPrivateAddressReturnsPublicAddress(c *C) {
	deploymentId := "b6de4c4c7d4a49c39270e0c57481fd9b"
	cleanup := overrideWALASharedConfig(c, deploymentId, "name", "10.76.200.59")
	defer cleanup()

	expectedAddress := deploymentId + ".cloudapp.net"
	prov := azureEnvironProvider{}
	pubAddress, err := prov.PrivateAddress()
	c.Assert(err, IsNil)
	c.Check(pubAddress, Equals, expectedAddress)
}

/*
func (EnvironProviderSuite) TestPrivateAddress(c *C) {
	internalAddress := "10.76.200.59"
	cleanup := overrideWALASharedConfig(c, "deploy-id", "name", internalAddress)
	defer cleanup()

	prov := azureEnvironProvider{}
	privAddress, err := prov.PrivateAddress()
	c.Assert(err, IsNil)
	c.Check(privAddress, Equals, internalAddress)
}
*/

func (EnvironProviderSuite) TestInstanceId(c *C) {
	deploymentName := "deploymentname"
	cleanup := overrideWALASharedConfig(c, "deploy-id", deploymentName, "10.76.200.59")
	defer cleanup()

	prov := azureEnvironProvider{}
	instanceId, err := prov.InstanceId()
	c.Assert(err, IsNil)
	c.Check(instanceId, Equals, instance.Id(deploymentName))
}
