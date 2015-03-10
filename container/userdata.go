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

	coreCloudinit "github.com/juju/juju/cloudinit"
	"github.com/juju/juju/environs/cloudinit"
)

var (
	logger = loggo.GetLogger("juju.container")
)

// WriteUserData generates the cloud-init user-data using the
// specified machine and network config for a container, and writes
// the serialized form out to a cloud-init file in the directory
// specified.
func WriteUserData(
	machineConfig *cloudinit.MachineConfig,
	networkConfig *NetworkConfig,
	directory string,
) (string, error) {
	userData, err := cloudInitUserData(machineConfig, networkConfig)
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
iface {{.InterfaceName}} inet static
    address {{.Address.Value}}
    netmask {{.CIDR}}{{if gt (len .DNSServers) 0}}
    dns-nameservers{{range $dns := .DNSServers}} {{$dns.Value}}{{end}}{{end}}
    pre-up ip route add {{.GatewayAddress.Value}} dev {{.InterfaceName}}
    pre-up ip route add default via {{.GatewayAddress.Value}}
    post-down ip route del default via {{.GatewayAddress.Value}}
    post-down ip route del {{.GatewayAddress.Value}} dev {{.InterfaceName}}
{{end}}{{define "dhcp"}}
{{.InterfaceName | printf "# interface %q"}}{{if not .NoAutoStart}}
auto {{.InterfaceName}}{{end}}
iface {{.InterfaceName}} inet dhcp
{{end}}{{range $nic := . }}{{if eq $nic.ConfigType "static"}}
{{template "static" $nic}}{{else}}{{template "dhcp" $nic}}{{end}}{{end}}`

var networkInterfacesFile = "/etc/network/interfaces"

// newCloudInitConfigWithNetworks creates a cloud-init config which
// might include per-interface networking config if
// networkConfig.Interfaces is not empty.
func newCloudInitConfigWithNetworks(networkConfig *NetworkConfig) (*coreCloudinit.Config, error) {
	cloudConfig := coreCloudinit.New()
	if networkConfig == nil || len(networkConfig.Interfaces) == 0 {
		// Don't generate networking config.
		logger.Tracef("no cloud-init network config to generate")
		return cloudConfig, nil
	}

	// Render the config first.
	tmpl, err := template.New("config").Parse(networkConfigTemplate)
	if err != nil {
		return nil, errors.Annotate(err, "cannot parse cloud-init network config template")
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, networkConfig.Interfaces); err != nil {
		return nil, errors.Annotate(err, "cannot render cloud-init network config")
	}

	// Now add it to cloud-init as a file created early in the boot process.
	cloudConfig.AddBootTextFile(networkInterfacesFile, buf.String(), 0644)
	return cloudConfig, nil
}

func cloudInitUserData(
	machineConfig *cloudinit.MachineConfig,
	networkConfig *NetworkConfig,
) ([]byte, error) {
	cloudConfig, err := newCloudInitConfigWithNetworks(networkConfig)
	udata, err := cloudinit.NewUserdataConfig(machineConfig, cloudConfig)
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

	renderer, err := coreCloudinit.NewRenderer(machineConfig.Series)
	if err != nil {
		return nil, err
	}

	data, err := renderer.Render(cloudConfig)
	if err != nil {
		return nil, err
	}
	return data, nil
}
