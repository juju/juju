// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/loggo"
	"os"
)

// Logger for the Azure provider.
var logger = loggo.GetLogger("juju.environs.azure")

type azureEnvironProvider struct{}

// azureEnvironProvider implements EnvironProvider.
var _ environs.EnvironProvider = (*azureEnvironProvider)(nil)

// Open is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Debugf("opening environment %q.", cfg.Name())
	return NewEnviron(cfg)
}

// PublicAddress is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) PublicAddress() (string, error) {
	config, err := parseWALAConfig()
	if err != nil {
		logger.Errorf("error parsing Windows Azure Linux Agent config file (%q): %v", _WALAConfigPath, err)
		return "", err
	}
	return config.getDeploymentHostname(), nil
}

// PrivateAddress is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) PrivateAddress() (string, error) {
	return prov.PublicAddress()
}

// InstanceId is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) InstanceId() (instance.Id, error) {
	config, err := parseWALAConfig()
	if err != nil {
		logger.Errorf("error parsing WALA config file (%q): %v", _WALAConfigPath, err)
		return instance.Id(""), err
	}
	return instance.Id(config.getDeploymentName()), nil
}

// Structures used to parse the XML Windows Azure Linux Agent (WALA)
// configuration file.
//
// Here is an example content of such a config file:
// <?xml version="1.0" encoding="utf-8"?>
// <SharedConfig version="1.0.0.0" goalStateIncarnation="1">
//   <Deployment name="b6de4c4c7d4a49c39270e0c57481fd9b" guid="{495985a8-8e5a-49aa-826f-d1f7f51045b6}" incarnation="0">
//    <Service name="gwaclmachineex95rsek" guid="{00000000-0000-0000-0000-000000000000}" />
//    <ServiceInstance name="b6de4c4c7d4a49c39270e0c57481fd9b.0" guid="{9806cac7-e566-42b8-9ecb-de8da8f69893}" />
//  [...]
//  </Deployment>
// </SharedConfig>

type WALASharedConfig struct {
	XMLName    xml.Name       `xml:SharedConfig`
	Deployment WALADeployment `xml:"Deployment"`
}

// getDeploymentName returns the deployment name referenced by the
// configuration.
// Confusingly, this is stored in the 'name' attribute of the 'Service'
// element.
func (config *WALASharedConfig) getDeploymentName() string {
	return config.Deployment.Service.Name
}

// getDeploymentHostname returns the hostname of this deployment.
// The hostname is taken from the 'name' attribute of the 'Deployment' element
// and the domain name is Azure's domain name: 'cloudapp.net'.
func (config *WALASharedConfig) getDeploymentHostname() string {
	return fmt.Sprintf("%s.cloudapp.net", config.Deployment.Name)
}

type WALADeployment struct {
	Name    string                `xml:"name,attr"`
	Service WALADeploymentService `xml:Service`
}

type WALADeploymentService struct {
	Name string `xml:"name,attr"`
}

// Path to the WALA configuration file.
var _WALAConfigPath = "/var/lib/waagent/SharedConfig.xml"

func parseWALAConfig() (*WALASharedConfig, error) {
	fin, err := os.Open(_WALAConfigPath)
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(fin)
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	config := &WALASharedConfig{}
	err = xml.Unmarshal([]byte(data), config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
