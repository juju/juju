// Copyright 2015 Cloudbase Solutions
// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/service/common"
)

var (
	logger   = loggo.GetLogger("juju.worker.deployer.service")
	renderer = &shell.PowershellRenderer{}

	// c_ERROR_SERVICE_DOES_NOT_EXIST is returned by the OS when trying to open
	// an inexistent service
	// https://msdn.microsoft.com/en-us/library/windows/desktop/ms684330%28v=vs.85%29.aspx
	c_ERROR_SERVICE_DOES_NOT_EXIST syscall.Errno = 0x424

	// c_ERROR_SERVICE_EXISTS is returned by the operating system if the service
	// we are trying to create, already exists
	c_ERROR_SERVICE_EXISTS syscall.Errno = 0x431

	// c_ERROR_ACCESS_DENIED is returned by the operating system if access is denied
	// to that service.
	c_ERROR_ACCESS_DENIED syscall.Errno = 0x5

	// This is the user under which juju services start. We chose to use a
	// normal user for this purpose because some installers require a normal
	// user with a proper user profile to actually run. This user is created
	// via userdata, and should exist on all juju bootstrapped systems.
	// Required privileges for this user are:
	// SeAssignPrimaryTokenPrivilege
	// SeServiceLogonRight
	jujudUser = ".\\jujud"
)

// IsRunning returns whether or not windows is the local init system.
func IsRunning() (bool, error) {
	return runtime.GOOS == "windows", nil
}

// ListServices returns the name of all installed services on the
// local host.
func ListServices() ([]string, error) {
	return listServices()
}

// ListCommand returns a command that will list the services on a host.
func ListCommand() string {
	return `(Get-Service).Name`
}

// ServiceManager exposes methods needed to manage a windows service
type ServiceManager interface {
	// Start starts a service.
	Start(name string) error
	// Stop stops a service.
	Stop(name string) error
	// Delete deletes a service.
	Delete(name string) error
	// Create creates a service with the given config.
	Create(name string, conf common.Conf) error
	// Running returns the status of a service.
	Running(name string) (bool, error)
	// Exists checks whether the config of the installed service matches the
	// config supplied to this function
	Exists(name string, conf common.Conf) (bool, error)
	// ChangeServicePassword can change the password of a service
	// as long as it belongs to the user defined in this package
	ChangeServicePassword(name, newPassword string) error
}

// Service represents a service running on the current system
type Service struct {
	common.Service
	manager ServiceManager
}

func newService(name string, conf common.Conf, manager ServiceManager) *Service {
	return &Service{
		Service: common.Service{
			Name: name,
			Conf: conf,
		},
		manager: manager,
	}
}

// NewService returns a new Service type
func NewService(name string, conf common.Conf) (*Service, error) {
	m, err := NewServiceManager()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newService(name, conf, m), nil
}

// Name implements service.Service.
func (s *Service) Name() string {
	return s.Service.Name
}

// Conf implements service.Service.
func (s *Service) Conf() common.Conf {
	return s.Service.Conf
}

// Validate checks the service for invalid values.
func (s *Service) Validate() error {
	if err := s.Service.Validate(renderer); err != nil {
		return errors.Trace(err)
	}

	if s.Service.Conf.Transient {
		return errors.NotSupportedf("transient services")
	}

	if s.Service.Conf.AfterStopped != "" {
		return errors.NotSupportedf("Conf.AfterStopped")
	}

	return nil
}

func (s *Service) Running() (bool, error) {
	if ok, err := s.Installed(); err != nil {
		return false, errors.Trace(err)
	} else if !ok {
		return false, nil
	}
	return s.manager.Running(s.Name())
}

// Installed returns whether the service is installed
func (s *Service) Installed() (bool, error) {
	services, err := ListServices()
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, val := range services {
		if s.Name() == val {
			return true, nil
		}
	}
	return false, nil
}

// Exists returns whether the service configuration reflects the
// desired state
func (s *Service) Exists() (bool, error) {
	return s.manager.Exists(s.Name(), s.Conf())
}

// Start starts the service.
func (s *Service) Start() error {
	logger.Infof("Starting service %q", s.Service.Name)
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if running {
		logger.Infof("Service %q already running", s.Service.Name)
		return nil
	}
	err = s.manager.Start(s.Name())
	return err
}

// Stop stops the service.
func (s *Service) Stop() error {
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if !running {
		return nil
	}
	err = s.manager.Stop(s.Name())
	return err
}

// Remove deletes the service.
func (s *Service) Remove() error {
	installed, err := s.Installed()
	if err != nil {
		return err
	}
	if !installed {
		return nil
	}

	err = s.Stop()
	if err != nil {
		return errors.Trace(err)
	}
	err = s.manager.Delete(s.Name())
	return err
}

// Install installs and starts the service.
func (s *Service) Install() error {
	err := s.Validate()
	if err != nil {
		return errors.Trace(err)
	}
	installed, err := s.Installed()
	if err != nil {
		return errors.Trace(err)
	}
	if installed {
		return errors.Errorf("Service %s already installed", s.Service.Name)
	}

	logger.Infof("Installing Service %v", s.Name)
	err = s.manager.Create(s.Name(), s.Conf())
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// InstallCommands returns shell commands to install the service.
func (s *Service) InstallCommands() ([]string, error) {
	cmd := fmt.Sprintf(serviceInstallCommands[1:],
		renderer.Quote(s.Service.Name),
		renderer.Quote(s.Service.Conf.Desc),
		renderer.Quote(s.Service.Conf.ExecStart),
		renderer.Quote(s.Service.Name),
		renderer.Quote(s.Service.Name),
	)
	return strings.Split(cmd, "\n"), nil
}

// StartCommands returns shell commands to start the service.
func (s *Service) StartCommands() ([]string, error) {
	cmd := fmt.Sprintf(`Start-Service %s`, renderer.Quote(s.Service.Name))
	return []string{cmd}, nil
}

const serviceInstallCommands = `
New-Service -Credential $jujuCreds -Name %s -DependsOn Winmgmt -DisplayName %s %s
sc.exe failure %s reset=5 actions=restart/1000
sc.exe failureflag %s 1`
