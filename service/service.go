package service

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
	"github.com/juju/juju/version"
)

var _ Service = (*upstart.Service)(nil)
var _ Service = (*windows.Service)(nil)

// Service represents a service running on the current system
type Service interface {
	// TODO(ericsnow) Add Name and Conf.

	// Installed will return a boolean value that denotes
	// whether or not the service is installed
	Installed() bool

	// Exists returns whether the service configuration exists in the
	// init directory with the same content that this Service would have
	// if installed.
	Exists() bool

	// Running returns a boolean value that denotes
	// whether or not the service is running
	Running() bool

	// Start will try to start the service
	Start() error

	// Stop will try to stop the service
	Stop() error

	// StopAndRemove will stop the service and remove it
	StopAndRemove() error

	// Remove will remove the service
	Remove() error

	// Install installs a service
	Install() error

	// Config adds a config to the service, overwritting the current one
	UpdateConfig(conf common.Conf)
}

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

// TODO(ericsnow) NewService -> DiscoverService.

// NewService returns an interface to a service apropriate
// for the current system
func NewService(name string, conf common.Conf) (Service, error) {
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

// TODO(ericsnow) Move windowsListServices to service/windows.ListServices.

func windowsListServices() ([]string, error) {
	com := exec.RunParams{
		Commands: `(Get-Service).Name`,
	}
	out, err := exec.RunCommands(com)
	if err != nil {
		return nil, err
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("Error running %s: %s", com.Commands, string(out.Stderr))
	}
	return strings.Fields(string(out.Stdout)), nil
}

// TODO(ericsnow) Move upstartListServices to service/upstart.ListServices.

var servicesRe = regexp.MustCompile("^([a-zA-Z0-9-_:]+)\\.conf$")

func upstartListServices(initDir string) ([]string, error) {
	var services []string
	fis, err := ioutil.ReadDir(initDir)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		if groups := servicesRe.FindStringSubmatch(fi.Name()); len(groups) > 0 {
			services = append(services, groups[1])
		}
	}
	return services, nil
}

// ListServices lists all installed services on the running system
func ListServices(initDir string) ([]string, error) {
	initName := versionInitSystem(version.Current)
	if initName == "" {
		return nil, errors.NotFoundf("init system on local host")
	}

	switch initName {
	case "windows":
		services, err := windowsListServices()
		return services, errors.Trace(err)
	case "upstart":
		services, err := upstartListServices(initDir)
		return services, errors.Trace(err)
	default:
		return nil, errors.NotFoundf("init system %q", initName)
	}
}
