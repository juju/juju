// Copyright 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package containerinit

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/proxy"

	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/container"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

var (
	logger = loggo.GetLogger("juju.cloudconfig.containerinit")
)

// WriteUserData generates the cloud-init user-data using the
// specified machine and network config for a container, and writes
// the serialized form out to a cloud-init file in the directory
// specified.
func WriteUserData(
	instanceConfig *instancecfg.InstanceConfig,
	networkConfig *container.NetworkConfig,
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
    dns-nameservers{{range $dns := .DNSServers}} {{$dns.Value}}{{end}}{{end}}{{if gt (len .DNSSearch) 0}}
    dns-search {{.DNSSearch}}{{end}}
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
func GenerateNetworkConfig(networkConfig *container.NetworkConfig) (string, error) {
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

// newCloudInitConfigWithNetworks creates a cloud-init config which
// might include per-interface networking config if both networkConfig
// is not nil and its Interfaces field is not empty.
func newCloudInitConfigWithNetworks(series string, networkConfig *container.NetworkConfig) (cloudinit.CloudConfig, error) {
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
	networkConfig *container.NetworkConfig,
) ([]byte, error) {
	cloudConfig, err := newCloudInitConfigWithNetworks(instanceConfig.Series, networkConfig)
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

	if instanceConfig.MachineContainerHostname != "" {
		cloudConfig.SetAttr("hostname", instanceConfig.MachineContainerHostname)
	}

	data, err := cloudConfig.RenderYAML()
	if err != nil {
		return nil, err
	}
	return data, nil
}

// templateUserData returns a minimal user data necessary for the template.
// This should have the authorized keys, base packages, the cloud archive if
// necessary,  initial apt proxy config, and it should do the apt-get
// update/upgrade initially.
func TemplateUserData(
	series string,
	authorizedKeys string,
	aptProxy proxy.Settings,
	aptMirror string,
	enablePackageUpdates bool,
	enableOSUpgrades bool,
	networkConfig *container.NetworkConfig,
) ([]byte, error) {
	var config cloudinit.CloudConfig
	var err error
	if networkConfig != nil {
		config, err = newCloudInitConfigWithNetworks(series, networkConfig)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		config, err = cloudinit.New(series)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	config.AddScripts(
		"set -xe", // ensure we run all the scripts or abort.
	)
	// For LTS series which need support for the cloud-tools archive,
	// we need to enable apt-get update regardless of the environ
	// setting, otherwise provisioning will fail.
	if series == "precise" && !enablePackageUpdates {
		logger.Warningf("series %q requires cloud-tools archive: enabling updates", series)
		enablePackageUpdates = true
	}

	config.AddSSHAuthorizedKeys(authorizedKeys)
	if enablePackageUpdates && config.RequiresCloudArchiveCloudTools() {
		config.AddCloudArchiveCloudTools()
	}
	config.AddPackageCommands(aptProxy, aptMirror, enablePackageUpdates, enableOSUpgrades)

	initSystem, err := service.VersionInitSystem(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cmds, err := shutdownInitCommands(initSystem, series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	config.AddScripts(strings.Join(cmds, "\n"))

	data, err := config.RenderYAML()
	if err != nil {
		return nil, err
	}
	return data, nil
}

// defaultEtcNetworkInterfaces is the contents of
// /etc/network/interfaces file which is left on the template LXC
// container on shutdown. This is needed to allow cloned containers to
// start in case no network config is provided during cloud-init, e.g.
// when AUFS is used or with the local provider (see bug #1431888).
const defaultEtcNetworkInterfaces = `
# loopback interface
auto lo
iface lo inet loopback

# primary interface
auto eth0
iface eth0 inet dhcp
`

func shutdownInitCommands(initSystem, series string) ([]string, error) {
	// These files are removed just before the template shuts down.
	cleanupOnShutdown := []string{
		// We remove any dhclient lease files so there's no chance a
		// clone to reuse a lease from the template it was cloned
		// from.
		"/var/lib/dhcp/dhclient*",
		// Both of these sets of files below are recreated on boot and
		// if we leave them in the template's rootfs boot logs coming
		// from cloned containers will be appended. It's better to
		// keep clean logs for diagnosing issues / debugging.
		"/var/log/cloud-init*.log",
	}

	// Using EOC below as the template shutdown script is itself
	// passed through cat > ... < EOF.
	replaceNetConfCmd := fmt.Sprintf(
		"/bin/cat > /etc/network/interfaces << EOC%sEOC\n  ",
		defaultEtcNetworkInterfaces,
	)
	paths := strings.Join(cleanupOnShutdown, " ")
	removeCmd := fmt.Sprintf("/bin/rm -fr %s\n  ", paths)
	shutdownCmd := "/sbin/shutdown -h now"
	name := "juju-template-restart"
	desc := "juju shutdown job"

	execStart := shutdownCmd
	if environs.AddressAllocationEnabled() {
		// Only do the cleanup and replacement of /e/n/i when address
		// allocation feature flag is enabled.
		execStart = replaceNetConfCmd + removeCmd + shutdownCmd
	}

	conf := common.Conf{
		Desc:         desc,
		Transient:    true,
		AfterStopped: "cloud-final",
		ExecStart:    execStart,
	}
	// systemd uses targets for synchronization of services
	if initSystem == service.InitSystemSystemd {
		conf.AfterStopped = "cloud-config.target"
	}

	svc, err := service.NewService(name, conf, series)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cmds, err := svc.InstallCommands()
	if err != nil {
		return nil, errors.Trace(err)
	}

	startCommands, err := svc.StartCommands()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cmds = append(cmds, startCommands...)

	return cmds, nil
}
