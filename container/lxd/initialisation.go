// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/utils/packaging/config"
	"github.com/juju/utils/packaging/manager"

	"github.com/juju/juju/container"
)

const lxdBridgeFile = "/etc/default/lxd-bridge"

var requiredPackages = []string{
	"lxd",
}

type containerInitialiser struct {
	series string
}

// containerInitialiser implements container.Initialiser.
var _ container.Initialiser = (*containerInitialiser)(nil)

// NewContainerInitialiser returns an instance used to perform the steps
// required to allow a host machine to run a LXC container.
func NewContainerInitialiser(series string) container.Initialiser {
	return &containerInitialiser{series}
}

// Initialise is specified on the container.Initialiser interface.
func (ci *containerInitialiser) Initialise() error {
	err := ensureDependencies(ci.series)
	if err != nil {
		return err
	}

	return configureLXDBridge()
}

// getPackageManager is a helper function which returns the
// package manager implementation for the current system.
func getPackageManager(series string) (manager.PackageManager, error) {
	return manager.NewPackageManager(series)
}

// getPackagingConfigurer is a helper function which returns the
// packaging configuration manager for the current system.
func getPackagingConfigurer(series string) (config.PackagingConfigurer, error) {
	return config.NewPackagingConfigurer(series)
}

func configureLXDBridge() error {
	f, err := os.OpenFile(lxdBridgeFile, os.O_RDWR, 0777)
	if err != nil {
		/* We're using an old version of LXD which doesn't have
		 * lxd-bridge; let's not fail here.
		 */
		if os.IsNotExist(err) {
			logger.Warningf("couldn't find %s, not configuring it", lxdBridgeFile)
			return nil
		}
		return errors.Trace(err)
	}
	defer f.Close()

	output, err := exec.Command("ip", "addr", "show").CombinedOutput()
	if err != nil {
		return errors.Trace(err)
	}

	subnet, err := detectSubnet(string(output))
	if err != nil {
		return errors.Trace(err)
	}

	existing, err := ioutil.ReadAll(f)
	if err != nil {
		return errors.Trace(err)
	}

	result := editLXDBridgeFile(string(existing), subnet)
	_, err = f.Seek(0, 0)
	if err != nil {
		return errors.Trace(err)
	}

	_, err = f.WriteString(result)
	if err != nil {
		return errors.Trace(err)
	}


	/* non-systemd systems don't have the lxd-bridge service, so this always fails */
	_ = exec.Command("service", "lxd-bridge", "restart").Run()
	return exec.Command("service", "lxd", "restart").Run()
}

func detectSubnet(ipAddrOutput string) (string, error) {
	max := 0

	for _, line := range strings.Split(ipAddrOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		columns := strings.Split(trimmed, " ")

		if len(columns) < 2 {
			return "", errors.Trace(fmt.Errorf("invalid ip addr output line %s", line))
		}

		if columns[0] != "inet" {
			continue
		}

		addr := columns[1]
		if !strings.HasPrefix(addr, "10.0.") {
			continue
		}

		tuples := strings.Split(addr, ".")
		if len(tuples) < 4 {
			return "", errors.Trace(fmt.Errorf("invalid ip addr %s", addr))
		}

		subnet, err := strconv.Atoi(tuples[2])
		if err != nil {
			return "", errors.Trace(err)
		}

		if subnet > max {
			max = subnet
		}
	}

	return fmt.Sprintf("%d", max+1), nil
}

func editLXDBridgeFile(input string, subnet string) string {
	buffer := bytes.Buffer{}

	for _, line := range strings.Split(input, "\n") {
		out := line

		if strings.HasPrefix(line, "USE_LXD_BRIDGE=") {
			out = `USE_LXD_BRIDGE="true"`
		} else if strings.HasPrefix(line, "EXISTING_BRIDGE=") {
			out = `EXISTING_BRIDGE=""`
		} else if strings.HasPrefix(line, "LXD_BRIDGE=") {
			out = `LXD_BRIDGE="lxdbr0"`
		} else if strings.HasPrefix(line, "LXD_IPV4_ADDR=") {
			out = fmt.Sprintf(`LXD_IPV4_ADDR="10.0.%s.1"`, subnet)
		} else if strings.HasPrefix(line, "LXD_IPV4_NETMASK=") {
			out = `LXD_IPV4_NETMASK="255.255.255.0"`
		} else if strings.HasPrefix(line, "LXD_IPV4_NETWORK=") {
			out = fmt.Sprintf(`LXD_IPV4_NETWORK="10.0.%s.1/24"`, subnet)
		} else if strings.HasPrefix(line, "LXD_IPV4_DHCP_RANGE=") {
			out = fmt.Sprintf(`LXD_IPV4_DHCP_RANGE="10.0.%s.2,10.0.%s.254"`, subnet, subnet)
		} else if strings.HasPrefix(line, "LXD_IPV4_DHCP_MAX=") {
			out = `LXD_IPV4_DHCP_MAX="253"`
		} else if strings.HasPrefix(line, "LXD_IPV4_NAT=") {
			out = `LXD_IPV4_NAT="true"`
		} else if strings.HasPrefix(line, "LXD_IPV6_PROXY=") {
			out = `LXD_IPV6_PROXY="false"`
		}

		buffer.WriteString(out)
		buffer.WriteString("\n")
	}

	return buffer.String()
}

// ensureDependencies creates a set of install packages using
// apt.GetPreparePackages and runs each set of packages through
// apt.GetInstall.
func ensureDependencies(series string) error {
	if series == "precise" {
		return fmt.Errorf("LXD is not supported in precise.")
	}

	pacman, err := getPackageManager(series)
	if err != nil {
		return err
	}
	pacconfer, err := getPackagingConfigurer(series)
	if err != nil {
		return err
	}

	for _, pack := range requiredPackages {
		pkg := pack
		if config.SeriesRequiresCloudArchiveTools(series) &&
			pacconfer.IsCloudArchivePackage(pack) {
			pkg = strings.Join(pacconfer.ApplyCloudArchiveTarget(pack), " ")
		}

		if config.RequiresBackports(series, pack) {
			pkg = fmt.Sprintf("--target-release %s-backports %s", series, pkg)
		}

		if err := pacman.Install(pkg); err != nil {
			return err
		}
	}

	return err
}
