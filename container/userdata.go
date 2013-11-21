// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container

import (
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"

	"launchpad.net/loggo"

	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/utils"
)

var (
	logger         = loggo.GetLogger("juju.container")
	aptHTTPProxyRE = regexp.MustCompile(`(?i)^Acquire::HTTP::Proxy\s+"([^"]+)";$`)
)

func WriteUserData(machineConfig *cloudinit.MachineConfig, directory string) (string, error) {
	userData, err := cloudInitUserData(machineConfig)
	if err != nil {
		logger.Errorf("failed to create user data: %v", err)
		return "", err
	}
	userDataFilename := filepath.Join(directory, "cloud-init")
	if err := ioutil.WriteFile(userDataFilename, userData, 0644); err != nil {
		logger.Errorf("failed to write user data: %v", err)
		return "", err
	}
	return userDataFilename, nil
}

func cloudInitUserData(machineConfig *cloudinit.MachineConfig) ([]byte, error) {
	// consider not having this line hardcoded...
	machineConfig.DataDir = "/var/lib/juju"
	cloudConfig := coreCloudinit.New()
	err := cloudinit.Configure(machineConfig, cloudConfig)
	if err != nil {
		return nil, err
	}

	// Run apt-config to fetch proxy settings from host. If no proxy
	// settings are configured, then we don't set up any proxy information
	// on the container.
	proxyConfig, err := utils.AptConfigProxy()
	if err != nil {
		return nil, err
	}
	if proxyConfig != "" {
		var proxyLines []string
		for _, line := range strings.Split(proxyConfig, "\n") {
			line = strings.TrimSpace(line)
			if len(line) > 0 {
				if m := aptHTTPProxyRE.FindStringSubmatch(line); m != nil {
					cloudConfig.SetAptProxy(m[1])
				} else {
					proxyLines = append(proxyLines, line)
				}
			}
		}
		if len(proxyLines) > 0 {
			cloudConfig.AddFile(
				"/etc/apt/apt.conf.d/99proxy-extra",
				strings.Join(proxyLines, "\n"),
				0644)
		}
	}

	// Run ifconfig to get the addresses of the internal container at least
	// logged in the host.
	cloudConfig.AddRunCmd("ifconfig")

	data, err := cloudConfig.Render()
	if err != nil {
		return nil, err
	}
	return data, nil
}
