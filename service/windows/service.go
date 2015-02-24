// Copyright 2014 Cloudbase Solutions
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"errors"
	"fmt"
	"strings"

	"github.com/juju/loggo"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/service/common"
)

var logger = loggo.GetLogger("juju.worker.deployer.service")

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

var serviceInstallScript = `$data = Get-Content "C:\Juju\Jujud.pass"
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
func (s *Service) Running() bool {
	status, err := s.Status()
	logger.Infof("Service %q Status %q", s.Service.Name, status)
	if err != nil {
		return false
	}
	if strings.TrimSpace(status) == "Stopped" {
		return false
	}
	return true
}

// Installed returns whether the service is installed
func (s *Service) Installed() bool {
	_, err := s.Status()
	if err == nil {
		return true
	}
	return false
}

// Exists returns whether the service configuration exists in the
// init directory with the same content that this Service would have
// if installed.
// TODO (gabriel-samfira): 2014-07-30 bug 1350171
// Needs a proper implementation when testing is improved
func (s *Service) Exists() bool {
	return false
}

// Start starts the service.
func (s *Service) Start() error {
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; Start-Service  "%s"`, s.Service.Name)
	logger.Infof("Starting service %q", s.Service.Name)
	if s.Running() {
		logger.Infof("Service %q already running", s.Service.Name)
		return nil
	}
	_, err := runPsCommand(cmd)
	return err
}

// Stop stops the service.
func (s *Service) Stop() error {
	if !s.Running() {
		return nil
	}
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; Stop-Service  "%s"`, s.Service.Name)
	_, err := runPsCommand(cmd)
	return err
}

// Remove deletes the service.
func (s *Service) Remove() error {
	_, err := s.Status()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; (gwmi win32_service -filter 'name="%s"').Delete()`, s.Service.Name)
	_, err = runPsCommand(cmd)
	return err
}

// StopAndRemove stops the service and then deletes the service.
func (s *Service) StopAndRemove() error {
	err := s.Stop()
	if err != nil {
		return err
	}
	return s.Remove()
}

// Install installs and starts the service.
func (s *Service) Install() error {
	err := s.Validate()
	if err != nil {
		return err
	}
	if s.Installed() {
		return errors.New(fmt.Sprintf("Service %s already installed", s.Name))
	}

	logger.Infof("Installing Service %v", s.Name)
	cmd := fmt.Sprintf(serviceInstallScript,
		s.Service.Name,
		s.Service.Conf.Desc,
		s.Service.Conf.ExecStart)
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

// InstallCommands returns shell commands to install and start the service.
func (s *Service) InstallCommands() ([]string, error) {
	cmd := fmt.Sprintf(serviceInstallScript,
		s.Service.Name,
		s.Service.Conf.Desc,
		s.Service.Conf.ExecStart)
	return strings.Split(cmd, "\n"), nil
}
