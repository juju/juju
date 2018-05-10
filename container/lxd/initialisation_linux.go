// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/packaging/config"
	"github.com/juju/utils/packaging/manager"
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/set"
	"github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/container"
	"github.com/juju/juju/network"
)

const lxdBridgeFile = "/etc/default/lxd-bridge"

var requiredPackages = []string{
	"lxd",
}

type containerInitialiser struct {
	series         string
	getExecCommand func(string, ...string) *exec.Cmd
}

// containerInitialiser implements container.Initialiser.
var _ container.Initialiser = (*containerInitialiser)(nil)

// NewContainerInitialiser returns an instance used to perform the steps
// required to allow a host machine to run a LXC container.
func NewContainerInitialiser(series string) container.Initialiser {
	return &containerInitialiser{
		series,
		exec.Command,
	}
}

// Initialise is specified on the container.Initialiser interface.
func (ci *containerInitialiser) Initialise() error {
	err := ensureDependencies(ci.series)
	if err != nil {
		return errors.Trace(err)
	}

	err = configureLXDBridge()
	if err != nil {
		return errors.Trace(err)
	}
	proxies := proxy.DetectProxies()
	err = ConfigureLXDProxies(proxies)
	if err != nil {
		return errors.Trace(err)
	}

	// LXD init is only run on Xenial and later.
	if ci.series == "trusty" {
		return nil
	}

	output, err := ci.getExecCommand(
		"lxd",
		"init",
		"--auto",
	).CombinedOutput()

	if err == nil {
		return nil
	}

	out := string(output)
	if strings.Contains(out, "You have existing containers or images. lxd init requires an empty LXD.") {
		// this error means we've already run lxd init, which is ok, so just
		// ignore it.
		return nil
	}

	return errors.Annotate(err, "while running lxd init --auto: "+out)
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
	setter, err := getLXDServerUpdater()
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(configureLXDProxies(setter, proxies))
}

var getLXDServerUpdater = getServerUpdaterConnect

func getServerUpdaterConnect() (serverUpdater, error) {
	return ConnectLocal()
}

type serverUpdater interface {
	GetServer() (server *api.Server, ETag string, err error)
	UpdateServer(server api.ServerPut, ETag string) (err error)
}

func configureLXDProxies(setter serverUpdater, proxies proxy.Settings) error {
	server, etag, err := setter.GetServer()
	if err != nil {
		return errors.Trace(err)
	}
	serverPut := server.Writable()
	serverPut.Config["core.proxy_http"] = proxies.Http
	serverPut.Config["core.proxy_https"] = proxies.Https
	serverPut.Config["core.proxy_ignore_hosts"] = proxies.NoProxy
	err = setter.UpdateServer(serverPut, etag)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// df returns the number of free bytes on the file system at the given path
var df = func(path string) (uint64, error) {
	// Note: do not use golang.org/x/sys/unix for this, it is
	// the best solution but will break the build in s390x
	// and introduce cgo dependency lp:1632541
	statfs := syscall.Statfs_t{}
	err := syscall.Statfs(path, &statfs)
	if err != nil {
		return 0, err
	}
	return uint64(statfs.Bsize) * statfs.Bfree, nil
}

var configureLXDBridge = func() error {
	server, err := ConnectLocal()
	if err != nil {
		return errors.Trace(err)
	}

	if err != nil {
		return errors.Trace(err)
	}

	// If LXD itself supports managing networks (added in LXD 2.3) we can allow
	// it to do all of the network configuration.
	if server.HasExtension("network") {
		return createDefaultBridgeInDefaultProfile(server)
	}
	return configureLXDBridgeForOlderLXD()
}

type networkClient interface {
	GetNetwork(name string) (network *api.Network, ETag string, err error)
	CreateNetwork(network api.NetworksPost) (err error)
	UpdateProfile(name string, profile api.ProfilePut, ETag string) (err error)
	GetProfile(name string) (profile *api.Profile, ETag string, err error)
}

func checkBridgeConfig(client networkClient, bridge string) error {
	n, _, err := client.GetNetwork(bridge)
	if err != nil {
		return errors.Annotatef(err, "LXD %s network config", bridge)
	}
	ipv6AddressConfig := n.Config["ipv6.address"]
	if n.Managed && ipv6AddressConfig != "none" && ipv6AddressConfig != "" {
		return errors.Errorf(`juju doesn't support ipv6. Please disable LXD's IPV6:

	$ lxc network set %s ipv6.address none

and rebootstrap`, bridge)
	}

	return nil
}

// CreateDefaultBridgeInDefaultProfile creates a default bridge if it doesn't
// exist and (if necessary) inserts it into the default profile.
func createDefaultBridgeInDefaultProfile(client networkClient) error {
	/* create the default bridge if it doesn't exist */
	n, _, err := client.GetNetwork(network.DefaultLXDBridge)
	if err != nil {
		networksPost := api.NetworksPost{
			Name: network.DefaultLXDBridge,
			NetworkPut: api.NetworkPut{
				Config: map[string]string{
					"ipv6.address": "none",
					"ipv6.nat":     "false",
				},
			},
		}
		err := client.CreateNetwork(networksPost)
		if err != nil {
			return errors.Trace(err)
		}

		n, _, err = client.GetNetwork(network.DefaultLXDBridge)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		if err := checkBridgeConfig(client, network.DefaultLXDBridge); err != nil {
			return errors.Trace(err)
		}
	}

	profile, etag, err := client.GetProfile("default")
	if err != nil {
		return errors.Trace(err)
	}
	config := profile.Writable()
	_, ok := config.Devices["eth0"]
	if ok {
		/* don't configure an eth0 if it already exists */
		return nil
	}

	nicType := "macvlan"
	if n.Type == "bridge" {
		nicType = "bridged"
	}

	config.Devices["eth0"] = map[string]string{
		"type":    "nic",
		"nictype": nicType,
		"parent":  network.DefaultLXDBridge,
	}

	err = client.UpdateProfile("default", config, etag)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// configureLXDBridgeForOlderLXD is used for LXD agents that don't support the
// Network API (pre 2.3)
func configureLXDBridgeForOlderLXD() error {
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
		return errors.Trace(err)
	}
	pacconfer, err := getPackagingConfigurer(series)
	if err != nil {
		return errors.Trace(err)
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
			return errors.Trace(err)
		}
	}

	return errors.Trace(err)
}

// randomizedOctetRange is a variable for testing purposes.
var randomizedOctetRange = func() []int {
	rand.Seed(time.Now().UnixNano())
	return rand.Perm(255)
}

// getKnownV4IPsAndCIDRs iterates all of the known Addresses on this machine
// and groups them up into known CIDRs and IP addresses.
func getKnownV4IPsAndCIDRs(addrFunc func() ([]net.Addr, error)) ([]net.IP, []*net.IPNet, error) {
	addrs, err := addrFunc()
	if err != nil {
		return nil, nil, errors.Annotate(err, "cannot get network interface addresses")
	}

	knownIPs := []net.IP{}
	seenIPs := set.NewStrings()
	knownCIDRs := []*net.IPNet{}
	seenCIDRs := set.NewStrings()
	for _, netAddr := range addrs {
		ip, ipNet, err := net.ParseCIDR(netAddr.String())
		if err != nil {
			continue
		}
		if ip.To4() == nil {
			continue
		}
		if !seenIPs.Contains(ip.String()) {
			knownIPs = append(knownIPs, ip)
			seenIPs.Add(ip.String())
		}
		if !seenCIDRs.Contains(ipNet.String()) {
			knownCIDRs = append(knownCIDRs, ipNet)
			seenCIDRs.Add(ipNet.String())
		}
	}
	return knownIPs, knownCIDRs, nil
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
	knownIPs, knownCIDRs, err := getKnownV4IPsAndCIDRs(interfaceAddrs)
	if err != nil {
		return "", errors.Trace(err)
	}

	randomized3rdSegment := randomizedOctetRange()
	for _, i := range randomized3rdSegment {
		// lxd randomizes the 2nd and 3rd segments, we should be fine with the
		// 3rd only
		ip, ip10network, err := net.ParseCIDR(fmt.Sprintf("10.0.%d.0/24", i))
		if err != nil {
			return "", errors.Trace(err)
		}

		collides := false
		for _, kIP := range knownIPs {
			if ip10network.Contains(kIP) {
				collides = true
				break
			}
		}
		if !collides {
			for _, kNet := range knownCIDRs {
				if kNet.Contains(ip) || ip10network.Contains(kNet.IP) {
					collides = true
					break
				}
			}
		}
		if !collides {
			return fmt.Sprintf("%d", i), nil
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
