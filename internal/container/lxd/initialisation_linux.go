// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"math/rand"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/juju/errors"
	"github.com/juju/os/v2/series"
	"github.com/juju/packaging/v2/manager"
	"github.com/juju/proxy"

	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/packaging"
	"github.com/juju/juju/internal/packaging/dependency"
	"github.com/juju/juju/internal/service"
)

var hostSeries = series.HostSeries

type containerInitialiser struct {
	containerNetworkingMethod string
	getExecCommand            func(string, ...string) *exec.Cmd
	configureLxdProxies       func(_ proxy.Settings, isRunningLocally func() (bool, error), newLocalServer func() (*Server, error)) error
	isRunningLocally          func() (bool, error)
	newLocalServer            func() (*Server, error)
	lxdSnapChannel            string
}

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/snap_manager_mock.go github.com/juju/juju/internal/container/lxd SnapManager

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
func NewContainerInitialiser(lxdSnapChannel, containerNetworkingMethod string) container.Initialiser {
	ci := &containerInitialiser{
		containerNetworkingMethod: containerNetworkingMethod,
		getExecCommand:            exec.Command,
		lxdSnapChannel:            lxdSnapChannel,
		isRunningLocally:          isRunningLocally,
		newLocalServer:            NewLocalServer,
	}
	ci.configureLxdProxies = internalConfigureLXDProxies
	return ci
}

// Initialise is specified on the container.Initialiser interface.
func (ci *containerInitialiser) Initialise() (err error) {
	localSeries, err := hostSeries()
	if err != nil {
		return errors.Trace(err)
	}

	if err := ensureDependencies(ci.lxdSnapChannel, localSeries); err != nil {
		return errors.Trace(err)
	}

	// We need to wait for LXD to be configured (via lxd init below)
	// before potentially updating proxy config for a local server.
	defer func() {
		if err == nil {
			proxies := proxy.DetectProxies()
			err = ci.configureLxdProxies(proxies, ci.isRunningLocally, ci.newLocalServer)
		}
	}()

	var output []byte
	if ci.containerNetworkingMethod == "local" {
		output, err = ci.getExecCommand(
			"lxd",
			"init",
			"--auto",
		).CombinedOutput()

		if err == nil {
			return nil
		}

	} else {

		lxdInitCfg := `config: {}
networks: []
storage_pools:
- config: {}
  description: ""
  name: default
  driver: dir
profiles:
- config: {}
  description: ""
  devices:
    root:
      path: /
      pool: default
      type: disk
  name: default
projects: []
cluster: null`

		cmd := ci.getExecCommand("lxd", "init", "--preseed")
		cmd.Stdin = strings.NewReader(lxdInitCfg)

		output, err = cmd.CombinedOutput()

		if err == nil {
			return nil
		}
	}

	out := string(output)
	if strings.Contains(out, "You have existing containers or images. lxd init requires an empty LXD.") {
		// this error means we've already run lxd init. Just ignore it.
		return nil
	}

	return errors.Annotate(err, "running lxd init: "+out)
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
	return IsUnixSocket("/var/snap/lxd/common/lxd/unix.socket")
}

// randomizedOctetRange is a variable for testing purposes.
var randomizedOctetRange = func() []int {
	rand.Seed(time.Now().UnixNano())
	return rand.Perm(255)
}

func isRunningLocally() (bool, error) {
	svcName, err := installedServiceName()
	if svcName == "" || err != nil {
		return false, errors.Trace(err)
	}

	svc, err := service.NewServiceReference(svcName)
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

	// Look for the Snap service.
	svcName := ""
	for _, name := range names {
		if name == "snap.lxd.daemon" {
			return name, nil
		}
	}
	return svcName, nil
}
