// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/packaging/config"
	"github.com/juju/utils/packaging/manager"
	"github.com/juju/utils/proxy"

	"github.com/juju/juju/container"
)

const lxdBridgeFile = "/etc/default/lxd-bridge"

var requiredPackages = []string{
	"lxd",
}

var xenialPackages = []string{
	"zfsutils-linux",
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

	err = configureLXDBridge()
	if err != nil {
		return err
	}
	proxies := proxy.DetectProxies()
	err = ConfigureLXDProxies(proxies)
	if err != nil {
		return err
	}

	// Well... this will need to change soon once we are passed 17.04 as who
	// knows what the series name will be.
	if ci.series >= "xenial" {
		configureZFS()
	}

	return nil
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

// ConfigureLXDProxies will try to set the lxc config core.proxy_http and core.proxy_https
// configuration values based on the current environment.
func ConfigureLXDProxies(proxies proxy.Settings) error {
	setter, err := getLXDConfigSetter()
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(configureLXDProxies(setter, proxies))
}

var getLXDConfigSetter = getConfigSetterConnect

func getConfigSetterConnect() (configSetter, error) {
	return ConnectLocal()
}

type configSetter interface {
	SetConfig(key, value string) error
}

func configureLXDProxies(setter configSetter, proxies proxy.Settings) error {
	err := setter.SetConfig("core.proxy_http", proxies.Http)
	if err != nil {
		return errors.Trace(err)
	}
	err = setter.SetConfig("core.proxy_https", proxies.Https)
	if err != nil {
		return errors.Trace(err)
	}
	err = setter.SetConfig("core.proxy_ignore_hosts", proxies.NoProxy)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

var execCommand = exec.Command

var configureZFS = func() {
	/* create a 100 GB pool by default (sparse, so it won't actually fill
	 * that immediately)
	 */
	output, err := execCommand(
		"lxd",
		"init",
		"--auto",
		"--storage-backend", "zfs",
		"--storage-pool", "lxd",
		"--storage-create-loop", "100",
	).CombinedOutput()

	if err != nil {
		logger.Errorf("configuring zfs failed with %s: %s", err, string(output))
	}
}

var configureLXDBridge = func() error {
	f, err := os.OpenFile(lxdBridgeFile, os.O_RDWR, 0777)
	if err != nil {
		/* We're using an old version of LXD which doesn't have
		 * lxd-bridge; let's not fail here.
		 */
		if os.IsNotExist(err) {
			logger.Debugf("couldn't find %s, not configuring it", lxdBridgeFile)
			return nil
		}
		return errors.Trace(err)
	}
	defer f.Close()

	existing, err := ioutil.ReadAll(f)
	if err != nil {
		return errors.Trace(err)
	}

	newBridgeCfg, err := bridgeConfiguration(string(existing))
	if err != nil {
		return errors.Trace(err)
	}

	if newBridgeCfg == string(existing) {
		return nil
	}

	_, err = f.Seek(0, 0)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = f.WriteString(newBridgeCfg)
	if err != nil {
		return errors.Trace(err)
	}

	/* non-systemd systems don't have the lxd-bridge service, so this always fails */
	_ = exec.Command("service", "lxd-bridge", "restart").Run()
	return exec.Command("service", "lxd", "restart").Run()
}

var interfaceAddrs = func() ([]net.Addr, error) {
	return net.InterfaceAddrs()
}

func editLXDBridgeFile(input string, subnet string) string {
	buffer := bytes.Buffer{}

	newValues := map[string]string{
		"USE_LXD_BRIDGE":      "true",
		"EXISTING_BRIDGE":     "",
		"LXD_BRIDGE":          "lxdbr0",
		"LXD_IPV4_ADDR":       fmt.Sprintf("10.0.%s.1", subnet),
		"LXD_IPV4_NETMASK":    "255.255.255.0",
		"LXD_IPV4_NETWORK":    fmt.Sprintf("10.0.%s.1/24", subnet),
		"LXD_IPV4_DHCP_RANGE": fmt.Sprintf("10.0.%s.2,10.0.%s.254", subnet, subnet),
		"LXD_IPV4_DHCP_MAX":   "253",
		"LXD_IPV4_NAT":        "true",
		"LXD_IPV6_PROXY":      "false",
	}
	found := map[string]bool{}

	for _, line := range strings.Split(input, "\n") {
		out := line

		for prefix, value := range newValues {
			if strings.HasPrefix(line, prefix+"=") {
				out = fmt.Sprintf(`%s="%s"`, prefix, value)
				found[prefix] = true
				break
			}
		}

		buffer.WriteString(out)
		buffer.WriteString("\n")
	}

	for prefix, value := range newValues {
		if !found[prefix] {
			buffer.WriteString(prefix)
			buffer.WriteString("=")
			buffer.WriteString(value)
			buffer.WriteString("\n")
			found[prefix] = true // not necessary but keeps "found" logically consistent
		}
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

	if series >= "xenial" {
		for _, pack := range xenialPackages {
			pacman.Install(fmt.Sprintf("--no-install-recommends %s", pack))
		}
	}

	return err
}

// findNextAvailableIPv4Subnet scans the list of interfaces on the machine
// looking for 10.0.0.0/16 networks and returns the next subnet not in
// use, having first detected the highest subnet. The next subnet can
// actually be lower if we overflowed 255 whilst seeking out the next
// unused subnet. If all subnets are in use an error is returned.
//
// TODO(frobware): this is not an ideal solution as it doesn't take
// into account any static routes that may be set up on the machine.
//
// TODO(frobware): this only caters for IPv4 setups.
func findNextAvailableIPv4Subnet() (string, error) {
	_, ip10network, err := net.ParseCIDR("10.0.0.0/16")
	if err != nil {
		return "", errors.Trace(err)
	}

	addrs, err := interfaceAddrs()
	if err != nil {
		return "", errors.Annotatef(err, "cannot get network interface addresses")
	}

	max := 0
	usedSubnets := make(map[int]bool)

	for _, address := range addrs {
		addr, network, err := net.ParseCIDR(address.String())
		if err != nil {
			logger.Debugf("cannot parse address %q: %v (ignoring)", address.String(), err)
			continue
		}
		if !ip10network.Contains(addr) {
			logger.Debugf("find available subnet, skipping %q", network.String())
			continue
		}
		subnet := int(network.IP[2])
		usedSubnets[subnet] = true
		if subnet > max {
			max = subnet
		}
	}

	if len(usedSubnets) == 0 {
		return "0", nil
	}

	for i := 0; i < 256; i++ {
		max = (max + 1) % 256
		if _, inUse := usedSubnets[max]; !inUse {
			return fmt.Sprintf("%d", max), nil
		}
	}

	return "", errors.New("could not find unused subnet")
}

func parseLXDBridgeConfigValues(input string) map[string]string {
	values := make(map[string]string)

	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}

		tokens := strings.Split(line, "=")

		if tokens[0] == "" {
			continue // no key
		}

		value := ""

		if len(tokens) > 1 {
			value = tokens[1]
			if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
				value = strings.Trim(value, `"`)
			}
		}

		values[tokens[0]] = value
	}
	return values
}

// bridgeConfiguration ensures that input has a valid setting for
// LXD_IPV4_ADDR, returning the existing input if is already set, and
// allocating the next available subnet if it is not.
func bridgeConfiguration(input string) (string, error) {
	values := parseLXDBridgeConfigValues(input)
	ipAddr := net.ParseIP(values["LXD_IPV4_ADDR"])

	if ipAddr == nil || ipAddr.To4() == nil {
		logger.Infof("LXD_IPV4_ADDR is not set; searching for unused subnet")
		subnet, err := findNextAvailableIPv4Subnet()
		if err != nil {
			return "", errors.Trace(err)
		}
		logger.Infof("setting LXD_IPV4_ADDR=10.0.%s.1", subnet)
		return editLXDBridgeFile(input, subnet), nil
	}
	return input, nil
}
