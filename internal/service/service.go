// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/retry"

	"github.com/juju/juju/internal/service/common"
	"github.com/juju/juju/internal/service/systemd"
)

var logger = loggo.GetLogger("juju.service")

const (
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

	// WriteService writes the service conf data. If the service is
	// running, WriteService adds links to allow for manual and automatic
	// starting of the service.
	WriteService() error
}

// RestartableService is a service that directly supports restarting.
type RestartableService interface {
	// Restart restarts the service.
	Restart() error
}

// NewService returns a new Service based on the provided info.
var NewService = func(name string, conf common.Conf) (Service, error) {
	if name == "" {
		return nil, errors.New("missing name")
	}
	return systemd.NewServiceWithDefaults(name, conf)
}

// NewServiceReference returns a reference to an existing Service
// and can be used to start/stop the service.
func NewServiceReference(name string) (Service, error) {
	return NewService(name, common.Conf{})
}

// ListServices lists all installed services on the running system
var ListServices = func() ([]string, error) {
	services, err := systemd.ListServices()
	return services, errors.Annotatef(err, "failed to list systemd services")
}

// ListServicesScript returns the commands that should be run to get
// a list of service names on a host.
func ListServicesScript() string {
	return systemd.ListCommand()
}

// installStartRetryAttempts defines how much InstallAndStart retries
// upon Start failures.
var installStartRetryStrategy = retry.CallArgs{
	Clock:       clock.WallClock,
	MaxDuration: 1 * time.Second,
	Delay:       250 * time.Millisecond,
}

// InstallAndStart installs the provided service and tries starting it.
// The first few Start failures are ignored.
func InstallAndStart(svc ServiceActions) error {
	service, ok := svc.(Service)
	if !ok {
		return errors.Errorf("specified service has no name %+v", svc)
	}
	logger.Infof("Installing and starting service %s", service.Name())
	logger.Debugf("Installing and starting service %+v", svc)

	if err := svc.Install(); err != nil {
		return errors.Trace(err)
	}

	// For various reasons the init system may take a short time to
	// realise that the service has been installed.
	retryStrategy := installStartRetryStrategy
	retryStrategy.Func = func() error { return manuallyRestart(svc) }
	retryStrategy.NotifyFunc = func(lastError error, _ int) {
		logger.Errorf("retrying start request (%v)", errors.Cause(lastError))
	}
	err := retry.Call(retryStrategy)
	if err != nil {
		err = retry.LastError(err)
		return errors.Trace(err)
	}
	return nil
}

// Restart restarts the named service.
func Restart(name string) error {
	svc, err := NewServiceReference(name)
	if err != nil {
		return errors.Annotatef(err, "failed to find service %q", name)
	}
	if err := manuallyRestart(svc); err != nil {
		return errors.Annotatef(err, "failed to restart service %q", name)
	}
	return nil
}

func manuallyRestart(svc ServiceActions) error {
	if err := svc.Stop(); err != nil {
		logger.Errorf("could not stop service: %v", err)
	}
	if err := svc.Start(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// FindAgents finds all the agents available on the machine.
func FindAgents(dataDir string) (string, []string, []string, error) {
	var (
		machineAgent  string
		unitAgents    []string
		errAgentNames []string
	)

	agentDir := filepath.Join(dataDir, "agents")

	entries, err := os.ReadDir(agentDir)
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
