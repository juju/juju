// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"math/rand"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/os/v2/series"
	"github.com/juju/packaging/v2/manager"
	"github.com/juju/proxy"
	"github.com/lxc/lxd/shared"

	"github.com/juju/juju/container"
	"github.com/juju/juju/packaging"
	"github.com/juju/juju/packaging/dependency"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

var hostSeries = series.HostSeries

type containerInitialiser struct {
	getExecCommand      func(string, ...string) *exec.Cmd
	configureLxdProxies func(_ proxy.Settings, isRunningLocally func() (bool, error), newLocalServer func() (*Server, error)) error
	configureLxdBridge  func() error
	isRunningLocally    func() (bool, error)
	newLocalServer      func() (*Server, error)
	lxdSnapChannel      string
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/snap_manager_mock.go github.com/juju/juju/container/lxd SnapManager

// SnapManager defines an interface implemented by types that can query and/or
// change the channel for installed snaps.
type SnapManager interface {
	InstalledChannel(string) string
	ChangeChannel(string, string) error
}

// getSnapManager returns a snap manager implementation that is used to query
// and/or change the channel for the installed lxd snap. Defined as a function
// so it can be overridden by tests.
var getSnapManager = func() SnapManager {
	return manager.NewSnapPackageManager()
}

// containerInitialiser implements container.Initialiser.
var _ container.Initialiser = (*containerInitialiser)(nil)

// NewContainerInitialiser returns an instance used to perform the steps
// required to allow a host machine to run a LXC container.
func NewContainerInitialiser(lxdSnapChannel string) container.Initialiser {
	ci := &containerInitialiser{
		getExecCommand:   exec.Command,
		lxdSnapChannel:   lxdSnapChannel,
		isRunningLocally: isRunningLocally,
		newLocalServer:   NewLocalServer,
	}
	ci.configureLxdBridge = ci.internalConfigureLXDBridge
	ci.configureLxdProxies = internalConfigureLXDProxies
	return ci
}

// Initialise is specified on the container.Initialiser interface.
func (ci *containerInitialiser) Initialise() error {
	localSeries, err := hostSeries()
	if err != nil {
		return errors.Trace(err)
	}

	if err := ensureDependencies(ci.lxdSnapChannel, localSeries); err != nil {
		return errors.Trace(err)
	}
	err = ci.configureLxdBridge()
	if err != nil {
		return errors.Trace(err)
	}
	proxies := proxy.DetectProxies()
	err = ci.configureLxdProxies(proxies, ci.isRunningLocally, ci.newLocalServer)
	if err != nil {
		return errors.Trace(err)
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
		// this error means we've already run lxd init. Just ignore it.
		return nil
	}

	return errors.Annotate(err, "running lxd init --auto: "+out)
}

// ConfigureLXDProxies will try to set the lxc config core.proxy_http and
// core.proxy_https configuration values based on the current environment.
// If LXD is not installed, we skip the configuration.
func ConfigureLXDProxies(proxies proxy.Settings) error {
	return internalConfigureLXDProxies(proxies, isRunningLocally, NewLocalServer)
}

func internalConfigureLXDProxies(
	proxies proxy.Settings,
	isRunningLocally func() (bool, error),
	newLocalServer func() (*Server, error),
) error {
	running, err := isRunningLocally()
	if err != nil {
		return errors.Trace(err)
	}

	if !running {
		logger.Debugf("LXD is not running; skipping proxy configuration")
		return nil
	}

	svr, err := newLocalServer()
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(svr.UpdateServerConfig(map[string]string{
		"core.proxy_http":         proxies.Http,
		"core.proxy_https":        proxies.Https,
		"core.proxy_ignore_hosts": proxies.NoProxy,
	}))
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

func (ci *containerInitialiser) internalConfigureLXDBridge() error {
	server, err := ci.newLocalServer()
	if err != nil {
		return errors.Trace(err)
	}
	// We do not support LXD versions without the network API,
	// which was added in 2.3.
	if !server.networkAPISupport {
		return errors.NotSupportedf("versions of LXD without network API")
	}

	profile, eTag, err := server.GetProfile(lxdDefaultProfileName)
	if err != nil {
		return errors.Trace(err)
	}
	// If there are no suitable bridged NICs in the profile,
	// ensure the bridge is set up and create one.
	if server.verifyNICsWithAPI(getProfileNICs(profile)) == nil {
		return nil
	}
	return server.ensureDefaultNetworking(profile, eTag)
}

var interfaceAddrs = func() ([]net.Addr, error) {
	return net.InterfaceAddrs()
}

// ensureDependencies install the required dependencies for running LXD.
func ensureDependencies(lxdSnapChannel, series string) error {
	// If the snap is already installed, check whether the operator asked
	// us to use a different channel. If so, switch to it.
	if lxdViaSnap() {
		snapManager := getSnapManager()
		trackedChannel := snapManager.InstalledChannel("lxd")
		// Note that images with pre-installed snaps are normally
		// tracking "latest/stable/ubuntu-$release_number". As our
		// default model config setting is "latest/stable", we perform
		// a starts-with check instead of an equality check to avoid
		// switching channels when we don't actually need to.
		if strings.HasPrefix(trackedChannel, lxdSnapChannel) {
			logger.Infof("LXD snap is already installed (channel: %s); skipping package installation", trackedChannel)
			return nil
		}

		// We need to switch to a different channel
		logger.Infof("switching LXD snap channel from %s to %s", trackedChannel, lxdSnapChannel)
		if err := snapManager.ChangeChannel("lxd", lxdSnapChannel); err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	if err := packaging.InstallDependency(dependency.LXD(lxdSnapChannel), series); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// lxdViaSnap interrogates the location of the Snap LXD socket in order
// to determine if LXD is being provided via that method.
var lxdViaSnap = func() bool {
	return shared.IsUnixSocket("/var/snap/lxd/common/lxd/unix.socket")
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

func isRunningLocally() (bool, error) {
	svcName, err := installedServiceName()
	if svcName == "" || err != nil {
		return false, errors.Trace(err)
	}

	hostSeries, err := series.HostSeries()
	if err != nil {
		return false, errors.Trace(err)
	}

	svc, err := service.NewService(svcName, common.Conf{}, hostSeries)
	if err != nil {
		return false, errors.Trace(err)
	}
	running, err := svc.Running()
	if err != nil {
		return running, errors.Trace(err)
	}
	return running, nil
}

// installedServiceName returns the name of the running service for the LXD
// daemon. If LXD is not installed, the return is an empty string.
func installedServiceName() (string, error) {
	names, err := service.ListServices()
	if err != nil {
		return "", errors.Trace(err)
	}

	// Prefer the Snap service.
	svcName := ""
	for _, name := range names {
		if name == "snap.lxd.daemon" {
			return name, nil
		}
		if name == "lxd" {
			svcName = name
		}
	}
	return svcName, nil
}
