package service

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
	"github.com/juju/juju/version"
)

var (
	logger = loggo.GetLogger("juju.service")
)

// These are the names of the init systems regognized by juju.
const (
	InitSystemWindows = "windows"
	InitSystemUpstart = "upstart"
	InitSystemSystemd = "systemd"
)

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

	// UpdateConfig adds a config to the service, overwriting the current one.
	UpdateConfig(conf common.Conf)

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

// TODO(ericsnow) bug #1426458
// Eliminate the need to pass an empty conf for most service methods
// and several helper functions.

// NewService returns a new Service based on the provided info.
func NewService(name string, conf common.Conf, initSystem string) (Service, error) {
	if name == "" {
		return nil, errors.New("missing name")
	}

	switch initSystem {
	case InitSystemWindows:
		return windows.NewService(name, conf), nil
	case InitSystemUpstart:
		return upstart.NewService(name, conf), nil
	case InitSystemSystemd:
		svc, err := systemd.NewService(name, conf)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to wrap service %q", name)
		}
		return svc, nil
	default:
		return nil, errors.NotFoundf("init system %q", initSystem)
	}
}

// ListServices lists all installed services on the running system
func ListServices() ([]string, error) {
	initName, ok := VersionInitSystem(version.Current)
	if !ok {
		return nil, errors.NotFoundf("init system on local host")
	}

	switch initName {
	case InitSystemWindows:
		services, err := windows.ListServices()
		if err != nil {
			return nil, errors.Annotatef(err, "failed to list %s services", initName)
		}
		return services, nil
	case InitSystemUpstart:
		services, err := upstart.ListServices()
		if err != nil {
			return nil, errors.Annotatef(err, "failed to list %s services", initName)
		}
		return services, nil
	case InitSystemSystemd:
		services, err := systemd.ListServices()
		if err != nil {
			return nil, errors.Annotatef(err, "failed to list %s services", initName)
		}
		return services, nil
	default:
		return nil, errors.NotFoundf("init system %q", initName)
	}
}

// ListServicesCommand returns the command that should be run to get
// a list of service names on a host.
func ListServicesCommand() string {
	return newShellSelectCommand(listServicesCommand)
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

// InstallStartRetryAttempts defines how much InstallAndStart retries
// upon Start failures.
var InstallStartRetryAttempts = utils.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 250 * time.Millisecond,
}

// InstallAndStart installs the provided service and tries starting it.
// The first few Start failures are ignored.
func InstallAndStart(svc ServiceActions) error {
	if err := svc.Install(); err != nil {
		return errors.Trace(err)
	}

	// For various reasons the init system may take a short time to
	// realise that the service has been installed.
	var err error
	for attempt := InstallStartRetryAttempts.Start(); attempt.Next(); {
		if err != nil {
			logger.Errorf("retrying start request (%v)", errors.Cause(err))
		}

		if err = svc.Start(); err == nil {
			break
		}
	}
	return errors.Trace(err)
}
