// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/os/series"
	"github.com/juju/utils"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/snap"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
)

var logger = loggo.GetLogger("juju.service")

// These are the names of the init systems recognized by juju.
const (
	InitSystemSystemd = "systemd"
	InitSystemUpstart = "upstart"
	InitSystemWindows = "windows"
	InitSystemSnap    = "snap"
)

// linuxInitSystems lists the names of the init systems that juju might
// find on a linux host.
var linuxInitSystems = []string{
	InitSystemSystemd,
	InitSystemUpstart,
	// InitSystemSnap is not part of this list, so that
	// the discovery machinery can't select snap over systemd
}

// ServiceActions represents the actions that may be requested for
// an init system service.
type ServiceActions interface {
	// Start will try to start the service.
	Start() error

	// Stop will try to stop the service.
	Stop() error

	// Install installs a service.
	Install() error

	// Remove will remove the service.
	Remove() error
}

// Service represents a service in the init system running on a host.
type Service interface {
	ServiceActions

	// Name returns the service's name.
	Name() string

	// Conf returns the service's conf data.
	Conf() common.Conf

	// Running returns a boolean value that denotes
	// whether or not the service is running.
	Running() (bool, error)

	// Exists returns whether the service configuration exists in the
	// init directory with the same content that this Service would have
	// if installed.
	Exists() (bool, error)

	// Installed will return a boolean value that denotes
	// whether or not the service is installed.
	Installed() (bool, error)

	// TODO(ericsnow) Move all the commands into a separate interface.

	// InstallCommands returns the list of commands to run on a
	// (remote) host to install the service.
	InstallCommands() ([]string, error)

	// StartCommands returns the list of commands to run on a
	// (remote) host to start the service.
	StartCommands() ([]string, error)
}

// ConfigurableService performs tasks that need to occur between the software
// has been installed and when has started
type ConfigurableService interface {
	// Configure performs any necessary configuration steps
	Configure() error

	// ReConfigureDuringRestart indicates whether Configure
	// should be called during a restart
	ReConfigureDuringRestart() bool
}

// RestartableService is a service that directly supports restarting.
type RestartableService interface {
	// Restart restarts the service.
	Restart() error
}

// UpgradableService describes a service that can be upgraded.
// It is assumed that such a service is not being upgraded across different
// init systems; rather taking a new form for the same init system.
type UpgradableService interface {
	// Remove old service deletes old files made obsolete by upgrade.
	RemoveOldService() error

	// WriteService writes the service conf data. If the service is
	// running, WriteService adds links to allow for manual and automatic
	// starting of the service.
	WriteService() error
}

// TODO(ericsnow) bug #1426458
// Eliminate the need to pass an empty conf for most service methods
// and several helper functions.

// NewService returns a new Service based on the provided info.
var NewService = func(name string, conf common.Conf, series string) (Service, error) {
	if name == "" {
		return nil, errors.New("missing name")
	}

	initSystem, err := versionInitSystem(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newService(name, conf, initSystem)
}

// this needs to be stubbed out in some tests
func newService(name string, conf common.Conf, initSystem string) (Service, error) {
	var svc Service
	var err error

	switch initSystem {
	case InitSystemWindows:
		svc, err = windows.NewService(name, conf)
	case InitSystemUpstart:
		svc, err = upstart.NewService(name, conf), nil
	case InitSystemSystemd:
		svc, err = systemd.NewServiceWithDefaults(name, conf)
	case InitSystemSnap:
		svc, err = snap.NewServiceFromName(name, conf)
	default:
		return nil, errors.NotFoundf("init system %q", initSystem)
	}

	if err != nil {
		return nil, errors.Annotatef(err, "failed to wrap service %q", name)
	}
	return svc, nil
}

// ListServices lists all installed services on the running system
var ListServices = func() ([]string, error) {
	hostSeries, err := series.HostSeries()
	if err != nil {
		return nil, errors.Trace(err)
	}
	initName, err := VersionInitSystem(hostSeries)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var services []string
	switch initName {
	case InitSystemWindows:
		services, err = windows.ListServices()
	case InitSystemSnap:
		services, err = snap.ListServices()
	case InitSystemUpstart:
		services, err = upstart.ListServices()
	case InitSystemSystemd:
		services, err = systemd.ListServices()
	default:
		return nil, errors.NotFoundf("init system %q", initName)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "failed to list %s services", initName)
	}
	return services, nil
}

// ListServicesScript returns the commands that should be run to get
// a list of service names on a host.
func ListServicesScript() string {
	commands := []string{
		"init_system=$(" + DiscoverInitSystemScript() + ")",
		// If the init system is not identified then the script will
		// "exit 1". This is correct since the script should fail if no
		// init system can be identified.
		newShellSelectCommand("init_system", "exit 1", listServicesCommand),
	}
	return strings.Join(commands, "\n")
}

func listServicesCommand(initSystem string) (string, bool) {
	switch initSystem {
	case InitSystemWindows:
		return windows.ListCommand(), true
	case InitSystemUpstart:
		return upstart.ListCommand(), true
	case InitSystemSystemd:
		return systemd.ListCommand(), true
	case InitSystemSnap:
		return snap.ListCommand(), true
	default:
		return "", false
	}
}

// installStartRetryAttempts defines how much InstallAndStart retries
// upon Start failures.
//
// TODO(katco): 2016-08-09: lp:1611427
var installStartRetryAttempts = utils.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 250 * time.Millisecond,
}

// InstallAndStart installs the provided service and tries starting it.
// The first few Start failures are ignored.
func InstallAndStart(svc ServiceActions) error {
	logger.Infof("Installing and starting service %+v", svc)
	if err := svc.Install(); err != nil {
		return errors.Trace(err)
	}

	// For various reasons the init system may take a short time to
	// realise that the service has been installed.
	var err error
	for attempt := installStartRetryAttempts.Start(); attempt.Next(); {
		if err != nil {
			logger.Errorf("retrying start request (%v)", errors.Cause(err))
		}
		// we attempt restart if the service is running in case daemon parameters
		// have changed, if its not running a regular start will happen.
		if err = ManuallyRestart(svc); err == nil {
			logger.Debugf("started %v", svc)
			break
		}
	}
	return errors.Trace(err)
}

// discoverService is patched out during some tests.
var discoverService = func(name string) (Service, error) {
	return DiscoverService(name, common.Conf{})
}

// TODO(ericsnow) Add one-off helpers for Start and Stop too?

// Restart restarts the named service.
func Restart(name string) error {
	svc, err := discoverService(name)
	if err != nil {
		return errors.Annotatef(err, "failed to find service %q", name)
	}
	if err := ManuallyRestart(svc); err != nil {
		return errors.Annotatef(err, "failed to restart service %q", name)
	}
	return nil
}

// ManuallyRestart restarts the service by applying
// its Restart method or by falling back to calling Stop and Start
func ManuallyRestart(svc ServiceActions) error {
	// TODO(tsm): fix service.upstart behaviour to match other implementations
	// if restartableService, ok := svc.(RestartableService); ok {
	// 	if err := restartableService.Restart(); err != nil {
	// 		return errors.Trace(err)
	// 	}
	// 	return nil
	// }

	if err := svc.Stop(); err != nil {
		logger.Errorf("could not stop service: %v", err)
	}
	configureableService, ok := svc.(ConfigurableService)
	if ok && configureableService.ReConfigureDuringRestart() {
		if err := configureableService.Configure(); err != nil {
			return errors.Trace(err)
		}
		return nil
	}
	if err := svc.Start(); err != nil {
		return errors.Trace(err)
	}
	return nil

}

// FindUnitServiceNames accepts a collection of service names as managed by the
// local init system. Any that are identified as being for unit agents are
// returned, keyed on the unit name.
func FindUnitServiceNames(svcNames []string) map[string]string {
	svcMatcher := regexp.MustCompile("^(jujud-.*unit-([a-z0-9-]+)-([0-9]+))$")
	unitServices := make(map[string]string)
	for _, svc := range svcNames {
		if groups := svcMatcher.FindStringSubmatch(svc); len(groups) > 0 {
			unitName := groups[2] + "/" + groups[3]
			if names.IsValidUnit(unitName) {
				unitServices[unitName] = groups[1]
			}
		}
	}
	return unitServices
}

// FindAgents finds all the agents available on the machine.
func FindAgents(dataDir string) (string, []string, []string, error) {
	var (
		machineAgent  string
		unitAgents    []string
		errAgentNames []string
	)

	agentDir := filepath.Join(dataDir, "agents")
	dir, err := os.Open(agentDir)
	if err != nil {
		return "", nil, nil, errors.Annotate(err, "opening agents dir")
	}
	defer func() { _ = dir.Close() }()

	entries, err := dir.Readdir(-1)
	if err != nil {
		return "", nil, nil, errors.Annotate(err, "reading agents dir")
	}
	for _, info := range entries {
		name := info.Name()
		tag, err := names.ParseTag(name)
		if err != nil {
			continue
		}
		switch tag.Kind() {
		case names.MachineTagKind:
			machineAgent = name
		case names.UnitTagKind:
			unitAgents = append(unitAgents, name)
		default:
			errAgentNames = append(errAgentNames, name)
			logger.Infof("%s is not of type Machine nor Unit, ignoring", name)
		}
	}
	return machineAgent, unitAgents, errAgentNames, nil
}
