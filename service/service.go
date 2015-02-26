// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/coreos/go-systemd/dbus"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/systemd"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/service/windows"
	"github.com/juju/juju/version"
)

// Service represents a service running on the current system
type Service interface {
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

// NewService returns an interface to a service apropriate
// for the current system
func NewService(name string, conf common.Conf) Service {
	switch version.Current.OS {
	case version.Ubuntu:
		return upstart.NewService(name, conf)
	case version.CentOS:
		return systemd.NewService(name, conf)
	case version.Windows:
		return windows.NewService(name, conf)
	default:
		return upstart.NewService(name, conf)
	}
}

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

func systemdListServices() ([]string, error) {
	var services []string

	conn, err := dbus.New()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	units, err := conn.ListUnits()
	if err != nil {
		return nil, err
	}

	for _, unit := range units {
		services = append(services, unit.Name)
	}

	return services, nil
}

// ListServices lists all installed services on the running system
func ListServices(initDir string) ([]string, error) {
	switch version.Current.OS {
	case version.Ubuntu:
		return upstartListServices(initDir)
	case version.CentOS:
		return systemdListServices()
	case version.Windows:
		return windowsListServices()
	default:
		return nil, fmt.Errorf("unrecognized OS version, cannot list services")
	}
}
