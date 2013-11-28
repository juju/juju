// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"io/ioutil"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/config"
)

type environProviderSuite struct {
	providerSuite
}

var _ = gc.Suite(&environProviderSuite{})

func (*environProviderSuite) TestOpen(c *gc.C) {
	prov := azureEnvironProvider{}
	attrs := makeAzureConfigMap(c)
	attrs["name"] = "my-shiny-new-env"
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	env, err := prov.Open(cfg)
	c.Assert(err, gc.IsNil)

	c.Check(env.Name(), gc.Equals, attrs["name"])
}

func (environProviderSuite) TestOpenReturnsNilInterfaceUponFailure(c *gc.C) {
	prov := azureEnvironProvider{}
	attrs := makeAzureConfigMap(c)
	// Make the config invalid.
	attrs["location"] = ""
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)

	env, err := prov.Open(cfg)
	// When Open() fails (i.e. returns a non-nil error), it returns an
	// environs.Environ interface object with a nil value and a nil
	// type.
	c.Check(env, gc.Equals, nil)
	c.Check(err, gc.ErrorMatches, ".*environment has no location; you need to set one.*")
}

// writeWALASharedConfig creates a temporary file with a valid WALinux config
// built using the given parameters. The file will be cleaned up at the end
// of the test calling this method.
func writeWALASharedConfig(c *gc.C, deploymentId string, deploymentName string, internalAddress string) string {
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
	c.Assert(err, gc.IsNil)
	filename := file.Name()
	err = ioutil.WriteFile(filename, []byte(config), 0644)
	c.Assert(err, gc.IsNil)
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
func overrideWALASharedConfig(c *gc.C, deploymentId, deploymentName, internalAddress string) func() {
	filename := writeWALASharedConfig(c, deploymentId, deploymentName,
		internalAddress)
	oldConfigPath := _WALAConfigPath
	_WALAConfigPath = filename
	// Return cleanup method to restore the original value of
	// '_WALAConfigPath'.
	return func() {
		_WALAConfigPath = oldConfigPath
	}
}

func (*environProviderSuite) TestParseWALASharedConfig(c *gc.C) {
	deploymentId := "b6de4c4c7d4a49c39270e0c57481fd9b"
	deploymentName := "gwaclmachineex95rsek"
	internalAddress := "10.76.200.59"

	cleanup := overrideWALASharedConfig(c, deploymentId, deploymentName, internalAddress)
	defer cleanup()

	config, err := parseWALAConfig()
	c.Assert(err, gc.IsNil)
	c.Check(config.Deployment.Name, gc.Equals, deploymentId)
	c.Check(config.Deployment.Service.Name, gc.Equals, deploymentName)
	c.Check(config.Instances[0].Address, gc.Equals, internalAddress)
}

func (*environProviderSuite) TestConfigGetDeploymentFQDN(c *gc.C) {
	deploymentId := "b6de4c4c7d4a49c39270e0c57481fd9b"
	serviceName := "gwaclr12slechtstschrijvende5"
	config := WALASharedConfig{
		Deployment: WALADeployment{
			Name:    deploymentId,
			Service: WALADeploymentService{Name: serviceName},
		},
	}

	c.Check(config.getDeploymentFQDN(), gc.Equals, serviceName+".cloudapp.net")
}

func (*environProviderSuite) TestConfigGetDeploymentHostname(c *gc.C) {
	deploymentName := "gwaclmachineex95rsek"
	config := WALASharedConfig{Deployment: WALADeployment{Name: "id", Service: WALADeploymentService{Name: deploymentName}}}

	c.Check(config.getDeploymentName(), gc.Equals, deploymentName)
}

func (*environProviderSuite) TestConfigGetInternalIP(c *gc.C) {
	internalAddress := "10.76.200.59"
	config := WALASharedConfig{Instances: []WALAInstance{{Address: internalAddress}}}

	c.Check(config.getInternalIP(), gc.Equals, internalAddress)
}

func (*environProviderSuite) TestPublicAddress(c *gc.C) {
	deploymentName := "b6de4c4c7d4a49c39270e0c57481fd9b"
	cleanup := overrideWALASharedConfig(c, "deploymentid", deploymentName, "10.76.200.59")
	defer cleanup()

	expectedAddress := deploymentName + ".cloudapp.net"
	prov := azureEnvironProvider{}
	pubAddress, err := prov.PublicAddress()
	c.Assert(err, gc.IsNil)
	c.Check(pubAddress, gc.Equals, expectedAddress)
}

func (*environProviderSuite) TestPrivateAddress(c *gc.C) {
	internalAddress := "10.76.200.59"
	cleanup := overrideWALASharedConfig(c, "deploy-id", "name", internalAddress)
	defer cleanup()

	prov := azureEnvironProvider{}
	privAddress, err := prov.PrivateAddress()
	c.Assert(err, gc.IsNil)
	c.Check(privAddress, gc.Equals, internalAddress)
}
