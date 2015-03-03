package service

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
	"github.com/juju/juju/version"
)

// These are the names of the init systems regognized by juju.
const (
	InitSystemWindows = "windows"
	InitSystemUpstart = "upstart"
	InitSystemSystemd = "systemd"
)

var _ Service = (*upstart.Service)(nil)
var _ Service = (*windows.Service)(nil)

// TODO(ericsnow) bug #1426461
// Running, Installed, and Exists should return errors.

// Service represents a service in the init system running on a host.
type Service interface {
	// Name returns the service's name.
	Name() string

	// Conf returns the service's conf data.
	Conf() common.Conf

	// UpdateConfig adds a config to the service, overwriting the current one.
	UpdateConfig(conf common.Conf)

	// Running returns a boolean value that denotes
	// whether or not the service is running.
	Running() bool

	// Start will try to start the service.
	Start() error

	// Stop will try to stop the service.
	Stop() error

	// TODO(ericsnow) Eliminate StopAndRemove.

	// StopAndRemove will stop the service and remove it.
	StopAndRemove() error

	// Exists returns whether the service configuration exists in the
	// init directory with the same content that this Service would have
	// if installed.
	Exists() bool

	// Installed will return a boolean value that denotes
	// whether or not the service is installed.
	Installed() bool

	// Install installs a service.
	Install() error

	// Remove will remove the service.
	Remove() error

	// InstallCommands returns the list of commands to run on a
	// (remote) host to install the service.
	InstallCommands() ([]string, error)
}

// TODO(ericsnow) bug #1426458
// Eliminate the need to pass an empty conf for most service methods
// and several helper functions.

// NewService returns a new Service based on the provided info.
func NewService(name string, conf common.Conf, initSystem string) (Service, error) {
	switch initSystem {
	case InitSystemWindows:
		return windows.NewService(name, conf), nil
	case InitSystemUpstart:
		return upstart.NewService(name, conf), nil
	case InitSystemSystemd:
		svc, err := systemd.NewService(name, conf)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return svc, nil
	default:
		return nil, errors.NotFoundf("init system %q", initSystem)
	}
}

// DiscoverService returns an interface to a service apropriate
// for the current system
func DiscoverService(name string, conf common.Conf) (Service, error) {
	initName, ok := VersionInitSystem(version.Current)
	if !ok {
		return nil, errors.NotFoundf("init system on local host")
	}

	service, err := NewService(name, conf, initName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return service, nil
}

// VersionInitSystem returns an init system name based on the provided
// version info. If one cannot be identified then false if returned
// for the second return value.
func VersionInitSystem(vers version.Binary) (string, bool) {
	switch vers.OS {
	case version.Windows:
		return InitSystemWindows, true
	case version.Ubuntu:
		switch vers.Series {
		case "precise", "quantal", "raring", "saucy", "trusty", "utopic":
			return InitSystemUpstart, true
		default:
			// vivid and later
			return InitSystemSystemd, true
		}
		// TODO(ericsnow) Support other OSes, like version.CentOS.
	default:
		return "", false
	}
}

// ListServices lists all installed services on the running system
func ListServices(initDir string) ([]string, error) {
	initName, ok := VersionInitSystem(version.Current)
	if !ok {
		return nil, errors.NotFoundf("init system on local host")
	}

	switch initName {
	case InitSystemWindows:
		services, err := windows.ListServices()
		if err != nil {
			return nil, err
		}
		return services, nil
	case InitSystemUpstart:
		services, err := upstart.ListServices(initDir)
		if err != nil {
			return nil, err
		}
		return services, nil
	case InitSystemSystemd:
		services, err := systemd.ListServices()
		if err != nil {
			return nil, err
		}
		return services, nil
	default:
		return nil, errors.NotFoundf("init system %q", initName)
	}
}

type initSystem struct {
	Name    string
	Flavour string
}

var linuxExecutables = []initSystem{
	// Note that some systems link /sbin/init to whatever init system
	// is supported, so in the future we may need some other way to
	// identify upstart uniquely.
	{"/sbin/init", InitSystemUpstart},
	{"/sbin/upstart", InitSystemUpstart},
	{"/sbin/systemd", InitSystemSystemd},
	{"/bin/systemd", InitSystemSystemd},
	{"/lib/systemd/systemd", InitSystemSystemd},
}

// TODO(ericsnow) Is it too much to cat once for each executable?
const initSystemTest = `[[ "$(cat /proc/1/cmdline)" == "%s" ]]`

// ListServicesCommand returns the command that should be run to get
// a list of service names on a host.
func ListServicesCommand() string {
	// TODO(ericsnow) Allow passing in "initSystems ...string".
	executables := linuxExecutables

	// TODO(ericsnow) build the command in a better way?

	cmdAll := ""
	for _, executable := range executables {
		cmd, ok := listServicesCommand(executable.Flavour)
		if !ok {
			continue
		}

		test := fmt.Sprintf(initSystemTest, executable.Name)
		cmd = fmt.Sprintf("if %s; then %s\n", test, cmd)
		if cmdAll != "" {
			cmd = "el" + cmd
		}
		cmdAll += cmd
	}
	if cmdAll != "" {
		cmdAll += "" +
			"else exit 1\n" +
			"fi"
	}
	return cmdAll
}

func listServicesCommand(initSystem string) (string, bool) {
	switch initSystem {
	case InitSystemWindows:
		return windows.ListCommand(), true
	case InitSystemUpstart:
		return upstart.ListCommand(), true
	case InitSystemSystemd:
		return systemd.ListCommand(), true
	default:
		return "", false
	}
}
