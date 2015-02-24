package service

import (
	"github.com/juju/errors"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
	"github.com/juju/juju/version"
)

var _ Service = (*upstart.Service)(nil)
var _ Service = (*windows.Service)(nil)

// Service represents a service running on the current system
type Service interface {
	// Name returns the service's name.
	Name() string

	// Conf returns the service's conf data.
	Conf() common.Conf

	// Config adds a config to the service, overwritting the current one
	UpdateConfig(conf common.Conf)

	// Running returns a boolean value that denotes
	// whether or not the service is running
	Running() bool

	// Start will try to start the service
	Start() error

	// Stop will try to stop the service
	Stop() error

	// TODO(ericsnow) Eliminate StopAndRemove.

	// StopAndRemove will stop the service and remove it
	StopAndRemove() error

	// Exists returns whether the service configuration exists in the
	// init directory with the same content that this Service would have
	// if installed.
	Exists() bool

	// Installed will return a boolean value that denotes
	// whether or not the service is installed
	Installed() bool

	// Install installs a service
	Install() error

	// Remove will remove the service
	Remove() error
}

// TODO(ericsnow) Eliminate the need to pass an empty conf here for
// most service methods.

func newService(name string, conf common.Conf, initSystem string) (Service, error) {
	var svc Service

	switch initSystem {
	case "windows":
		svc = windows.NewService(name, conf)
	case "upstart":
		svc = upstart.NewService(name, conf)
	default:
		return nil, errors.NotFoundf("init system %q", initSystem)
	}

	return svc, nil
}

// DiscoverService returns an interface to a service apropriate
// for the current system
func DiscoverService(name string, conf common.Conf) (Service, error) {
	initName := versionInitSystem(version.Current)
	if initName == "" {
		return nil, errors.NotFoundf("init system on local host")
	}

	service, err := newService(name, conf, initName)
	return service, errors.Trace(err)
}

func versionInitSystem(vers version.Binary) string {
	switch vers.OS {
	case version.Windows:
		return "windows"
	case version.Ubuntu:
		switch vers.Series {
		case "precise", "quantal", "raring", "saucy", "trusty", "utopic":
			return "upstart"
		default:
			// vivid and later
			return "systemd"
		}
		// TODO(ericsnow) Support other OSes, like version.CentOS.
	default:
		return ""
	}
}

// ListServices lists all installed services on the running system
func ListServices(initDir string) ([]string, error) {
	initName := versionInitSystem(version.Current)
	if initName == "" {
		return nil, errors.NotFoundf("init system on local host")
	}

	switch initName {
	case "windows":
		services, err := windows.ListServices()
		return services, errors.Trace(err)
	case "upstart":
		services, err := upstart.ListServices(initDir)
		return services, errors.Trace(err)
	default:
		return nil, errors.NotFoundf("init system %q", initName)
	}
}
