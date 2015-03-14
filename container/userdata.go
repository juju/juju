// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"text/template"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
)

var (
	logger = loggo.GetLogger("juju.container")
)

// WriteUserData generates the cloud-init user-data using the
// specified machine and network config for a container, and writes
// the serialized form out to a cloud-init file in the directory
// specified.
func WriteUserData(
	instanceConfig *instancecfg.InstanceConfig,
	networkConfig *NetworkConfig,
	directory string,
) (string, error) {
	userData, err := cloudInitUserData(instanceConfig, networkConfig)
	if err != nil {
		logger.Errorf("failed to create user data: %v", err)
		return "", err
	}
	return WriteCloudInitFile(directory, userData)
}

// WriteCloudInitFile writes the data out to a cloud-init file in the
// directory specified, and returns the filename.
func WriteCloudInitFile(directory string, userData []byte) (string, error) {
	userDataFilename := filepath.Join(directory, "cloud-init")
	if err := ioutil.WriteFile(userDataFilename, userData, 0644); err != nil {
		logger.Errorf("failed to write user data: %v", err)
		return "", err
	}
	return userDataFilename, nil
}

// networkConfigTemplate defines how to render /etc/network/interfaces
// file for a container with one or more NICs.
const networkConfigTemplate = `
# loopback interface
auto lo
iface lo inet loopback{{define "static"}}
{{.InterfaceName | printf "# interface %q"}}{{if not .NoAutoStart}}
auto {{.InterfaceName}}{{end}}
iface {{.InterfaceName}} inet manual{{if gt (len .DNSServers) 0}}
    dns-nameservers{{range $dns := .DNSServers}} {{$dns.Value}}{{end}}{{end}}
    pre-up ip address add {{.Address.Value}}/32 dev {{.InterfaceName}} &> /dev/null || true
    up ip route replace {{.GatewayAddress.Value}} dev {{.InterfaceName}}
    up ip route replace default via {{.GatewayAddress.Value}}
    down ip route del default via {{.GatewayAddress.Value}} &> /dev/null || true
    down ip route del {{.GatewayAddress.Value}} dev {{.InterfaceName}} &> /dev/null || true
    post-down ip address del {{.Address.Value}}/32 dev {{.InterfaceName}} &> /dev/null || true
{{end}}{{define "dhcp"}}
{{.InterfaceName | printf "# interface %q"}}{{if not .NoAutoStart}}
auto {{.InterfaceName}}{{end}}
iface {{.InterfaceName}} inet dhcp
{{end}}{{range $nic := . }}{{if eq $nic.ConfigType "static"}}
{{template "static" $nic}}{{else}}{{template "dhcp" $nic}}{{end}}{{end}}`

var networkInterfacesFile = "/etc/network/interfaces"

// GenerateNetworkConfig renders a network config for one or more
// network interfaces, using the given non-nil networkConfig
// containing a non-empty Interfaces field.
func GenerateNetworkConfig(networkConfig *NetworkConfig) (string, error) {
	if networkConfig == nil || len(networkConfig.Interfaces) == 0 {
		// Don't generate networking config.
		logger.Tracef("no network config to generate")
		return "", nil
	}

	// Render the config first.
	tmpl, err := template.New("config").Parse(networkConfigTemplate)
	if err != nil {
		return "", errors.Annotate(err, "cannot parse network config template")
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, networkConfig.Interfaces); err != nil {
		return "", errors.Annotate(err, "cannot render network config")
	}

	return buf.String(), nil
}

// NewCloudInitConfigWithNetworks creates a cloud-init config which
// might include per-interface networking config if both networkConfig
// is not nil and its Interfaces field is not empty.
func NewCloudInitConfigWithNetworks(series string, networkConfig *NetworkConfig) (cloudinit.CloudConfig, error) {
	cloudConfig, err := cloudinit.New(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	config, err := GenerateNetworkConfig(networkConfig)
	if err != nil || len(config) == 0 {
		return cloudConfig, errors.Trace(err)
	}

	// Now add it to cloud-init as a file created early in the boot process.
	cloudConfig.AddBootTextFile(networkInterfacesFile, config, 0644)
	return cloudConfig, nil
}

func cloudInitUserData(
	instanceConfig *instancecfg.InstanceConfig,
	networkConfig *NetworkConfig,
) ([]byte, error) {
	cloudConfig, err := NewCloudInitConfigWithNetworks(instanceConfig.Series, networkConfig)
	udata, err := cloudconfig.NewUserdataConfig(instanceConfig, cloudConfig)
	if err != nil {
		return nil, err
	}
	err = udata.Configure()
	if err != nil {
		return nil, err
	}
	// Run ifconfig to get the addresses of the internal container at least
	// logged in the host.
	cloudConfig.AddRunCmd("ifconfig")

	renderer, err := cloudinit.NewRenderer(instanceConfig.Series)
	if err != nil {
		return nil, err
	}

	data, err := renderer.Render(cloudConfig)
	if err != nil {
		return nil, err
	}
	return data, nil
}
