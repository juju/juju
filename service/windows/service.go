// Copyright 2014 Cloudbase Solutions
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/shell"

	"github.com/juju/juju/service/common"
)

var (
	logger = loggo.GetLogger("juju.worker.deployer.service")

	renderer = &shell.PowershellRenderer{}
)

// IsRunning returns whether or not windows is the local init system.
func IsRunning() (bool, error) {
	return runtime.GOOS == "windows", nil
}

// ListServices returns the name of all installed services on the
// local host.
func ListServices() ([]string, error) {
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

// ListCommand returns a command that will list the services on a host.
func ListCommand() string {
	return `(Get-Service).Name`
}

// Service represents a service running on the current system
type Service struct {
	common.Service
}

func runPsCommand(cmd string) (*exec.ExecResponse, error) {
	com := exec.RunParams{
		Commands: cmd,
	}
	out, err := exec.RunCommands(com)
	if err != nil {
		return nil, err
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("Error running %s: %s", cmd, string(out.Stderr))
	}
	return out, nil
}

// Name implements service.Service.
func (s Service) Name() string {
	return s.Service.Name
}

// Conf implements service.Service.
func (s Service) Conf() common.Conf {
	return s.Service.Conf
}

// Validate checks the service for invalid values.
func (s Service) Validate() error {
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

// Status gets the service status
func (s *Service) Status() (string, error) {
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; (Get-Service "%s").Status`, s.Service.Name)
	out, err := runPsCommand(cmd)
	if err != nil {
		return "", err
	}
	return string(out.Stdout), nil
}

// Running returns true if the Service appears to be running.
func (s *Service) Running() (bool, error) {
	installed, err := s.Installed()
	if err != nil {
		return false, err
	}
	if !installed {
		return false, nil
	}
	status, err := s.Status()
	logger.Infof("Service %q Status %q", s.Service.Name, status)
	if err != nil {
		return false, errors.Trace(err)
	}
	if strings.TrimSpace(status) == "Stopped" {
		return false, nil
	}
	return true, nil
}

// Installed returns whether the service is installed
func (s *Service) Installed() (bool, error) {
	services, err := ListServices()
	if err != nil {
		return false, err
	}
	for _, val := range services {
		if val == s.Name() {
			return true, nil
		}
	}
	return false, nil
}

// Exists returns whether the service configuration exists in the
// init directory with the same content that this Service would have
// if installed.
// TODO (gabriel-samfira): 2014-07-30 bug 1350171
// Needs a proper implementation when testing is improved
func (s *Service) Exists() (bool, error) {
	return false, nil
}

// Start starts the service.
func (s *Service) Start() error {
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; Start-Service  "%s"`, s.Service.Name)
	logger.Infof("Starting service %q", s.Service.Name)
	running, err := s.Running()
	if err != nil {
		return errors.Trace(err)
	}
	if running {
		logger.Infof("Service %q already running", s.Service.Name)
		return nil
	}
	_, err = runPsCommand(cmd)
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
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; Stop-Service  "%s"`, s.Service.Name)
	_, err = runPsCommand(cmd)
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
		return err
	}
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; (gwmi win32_service -filter 'name="%s"').Delete()`, s.Service.Name)
	_, err = runPsCommand(cmd)
	return err
}

// Install installs and starts the service.
func (s *Service) Install() error {
	err := s.Validate()
	if err != nil {
		return err
	}
	installed, err := s.Installed()
	if err != nil {
		return errors.Trace(err)
	}
	if installed {
		return errors.New(fmt.Sprintf("Service %s already installed", s.Service.Name))
	}

	logger.Infof("Installing Service %v", s.Name)
	cmd := fmt.Sprintf(serviceInstallScript[1:],
		s.Service.Name,
		s.Service.Conf.Desc,
		s.Service.Conf.ExecStart,
	)
	outCmd, errCmd := runPsCommand(cmd)

	if errCmd != nil {
		logger.Infof("ERROR installing service %v --> %v", outCmd, errCmd)
		return errCmd
	}
	return s.Start()
}

// NewService returns a new Service type
func NewService(name string, conf common.Conf) *Service {
	return &Service{
		Service: common.Service{
			Name: name,
			Conf: conf,
		},
	}
}

// InstallCommands returns shell commands to install the service.
func (s *Service) InstallCommands() ([]string, error) {
	cmd := fmt.Sprintf(serviceInstallCommands[1:],
		renderer.Quote(s.Service.Name),
		renderer.Quote(s.Service.Conf.Desc),
		renderer.Quote(s.Service.Conf.ExecStart),
		renderer.Quote(s.Service.Name),
	)
	return strings.Split(cmd, "\n"), nil
}

// StartCommands returns shell commands to start the service.
func (s *Service) StartCommands() ([]string, error) {
	// TODO(ericsnow) Merge with the command in Start().
	cmd := fmt.Sprintf(`Start-Service %s`, renderer.Quote(s.Service.Name))
	return []string{cmd}, nil
}

// TODO(ericsnow) Merge serviceInstallCommands and serviceInstallScript?

const serviceInstallCommands = `
New-Service -Credential $jujuCreds -Name %s -DisplayName %s %s
cmd.exe /C sc config %s start=delayed-auto`

const serviceInstallScript = `
$data = Get-Content "C:\Juju\Jujud.pass"
if($? -eq $false){Write-Error "Failed to read encrypted password"; exit 1}
$serviceName = "%s"
$secpasswd = $data | convertto-securestring
if($? -eq $false){Write-Error "Failed decode password"; exit 1}
$juju_user = whoami
$jujuCreds = New-Object System.Management.Automation.PSCredential($juju_user, $secpasswd)
if($? -eq $false){Write-Error "Failed to create secure credentials"; exit 1}
New-Service -Credential $jujuCreds -Name "$serviceName" -DisplayName '%s' '%s'
if($? -eq $false){Write-Error "Failed to install service $serviceName"; exit 1}
cmd.exe /C call sc config $serviceName start=delayed-auto
if($? -eq $false){Write-Error "Failed execute sc"; exit 1}
`
