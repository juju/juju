// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
)

// Register the Azure provider with Juju.
func init() {
	environs.RegisterProvider("azure", azureEnvironProvider{})
}

// Logger for the Azure provider.
var logger = loggo.GetLogger("juju.provider.azure")

type azureEnvironProvider struct{}

// azureEnvironProvider implements EnvironProvider.
var _ environs.EnvironProvider = (*azureEnvironProvider)(nil)

// Open is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Debugf("opening environment %q.", cfg.Name())
	// We can't return NewEnviron(cfg) directly here because otherwise,
	// when err is not nil, we end up with a non-nil returned environ and
	// this breaks the loop in cmd/jujud/upgrade.go:run() (see
	// http://golang.org/doc/faq#nil_error for the gory details).
	environ, err := NewEnviron(cfg)
	if err != nil {
		return nil, err
	}
	return environ, nil
}

// Prepare is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) Prepare(cfg *config.Config) (environs.Environ, error) {
	// TODO prepare environment as necessary
	return prov.Open(cfg)
}

// PublicAddress is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) PublicAddress() (string, error) {
	config, err := parseWALAConfig()
	if err != nil {
		logger.Errorf("error parsing Windows Azure Linux Agent config file (%q): %v", _WALAConfigPath, err)
		return "", err
	}
	return config.getDeploymentFQDN(), nil
}

// PrivateAddress is specified in the EnvironProvider interface.
func (prov azureEnvironProvider) PrivateAddress() (string, error) {
	config, err := parseWALAConfig()
	if err != nil {
		logger.Errorf("error parsing Windows Azure Linux Agent config file (%q): %v", _WALAConfigPath, err)
		return "", err
	}
	return config.getInternalIP(), nil
}

// The XML Windows Azure Linux Agent (WALA) is the agent which runs on all
// the Linux Azure VMs.  The hostname of the VM is the service name and the
// juju instanceId is (by design), the deployment's name.
//
// See https://github.com/windows-azure/walinuxagent for more details.
//
// Here is an example content of such a config file:
// <?xml version="1.0" encoding="utf-8"?>
// <SharedConfig version="1.0.0.0" goalStateIncarnation="1">
//   <Deployment name="b6de4c4c7d4a49c39270e0c57481fd9b" guid="{495985a8-8e5a-49aa-826f-d1f7f51045b6}" incarnation="0">
//    <Service name="gwaclmachineex95rsek" guid="{00000000-0000-0000-0000-000000000000}" />
//    <ServiceInstance name="b6de4c4c7d4a49c39270e0c57481fd9b.0" guid="{9806cac7-e566-42b8-9ecb-de8da8f69893}" />
//  [...]
//  <Instances>
//    <Instance id="gwaclroleldc1o5p" address="10.76.200.59">
//      [...]
//    </Instance>
//  </Instances>
//  </Deployment>
// </SharedConfig>

// Structures used to parse the XML Windows Azure Linux Agent (WALA)
// configuration file.

type WALASharedConfig struct {
	XMLName    xml.Name       `xml:"SharedConfig"`
	Deployment WALADeployment `xml:"Deployment"`
	Instances  []WALAInstance `xml:"Instances>Instance"`
}

// getDeploymentName returns the deployment name referenced by the
// configuration.
// Confusingly, this is stored in the 'name' attribute of the 'Service'
// element.
func (config *WALASharedConfig) getDeploymentName() string {
	return config.Deployment.Service.Name
}

// getDeploymentFQDN returns the FQDN of this deployment.
// The hostname is taken from the 'name' attribute of the Service element
// embedded in the Deployment element.  The domain name is Azure's fixed
// domain name: 'cloudapp.net'.
func (config *WALASharedConfig) getDeploymentFQDN() string {
	return fmt.Sprintf("%s.%s", config.getDeploymentName(), AZURE_DOMAIN_NAME)
}

// getInternalIP returns the internal IP for this deployment.
// The internalIP is the internal IP of the only instance in this deployment.
func (config *WALASharedConfig) getInternalIP() string {
	return config.Instances[0].Address
}

type WALADeployment struct {
	Name    string                `xml:"name,attr"`
	Service WALADeploymentService `xml:"Service"`
}

type WALADeploymentService struct {
	Name string `xml:"name,attr"`
}

type WALAInstance struct {
	Address string `xml:"address,attr"`
}

// Path to the WALA configuration file.
var _WALAConfigPath = "/var/lib/waagent/SharedConfig.xml"

func parseWALAConfig() (*WALASharedConfig, error) {
	data, err := ioutil.ReadFile(_WALAConfigPath)
	if err != nil {
		return nil, err
	}
	config := &WALASharedConfig{}
	err = xml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
