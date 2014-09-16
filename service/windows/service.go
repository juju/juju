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
	Name string
	Conf common.Conf
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

func (s *Service) UpdateConfig(conf common.Conf) {
	s.Conf = conf
}

// Status gets the service status
func (s *Service) Status() (string, error) {
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; (Get-Service "%s").Status`, s.Name)
	out, err := runPsCommand(cmd)
	if err != nil {
		return "", err
	}
	return string(out.Stdout), nil
}

// Running returns true if the Service appears to be running.
func (s *Service) Running() bool {
	status, err := s.Status()
	logger.Infof("Service %q Status %q", s.Name, status)
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
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; Start-Service  "%s"`, s.Name)
	logger.Infof("Starting service %q", s.Name)
	if s.Running() {
		logger.Infof("Service %q already running", s.Name)
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
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; Stop-Service  "%s"`, s.Name)
	_, err := runPsCommand(cmd)
	return err
}

// Remove deletes the service.
func (s *Service) Remove() error {
	_, err := s.Status()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; (gwmi win32_service -filter 'name="%s"').Delete()`, s.Name)
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

func (s *Service) validate() error {
	if s.Conf.Cmd == "" {
		return errors.New("missing Cmd")
	}
	if s.Conf.Desc == "" {
		return errors.New("missing Description")
	}
	if s.Name == "" {
		return errors.New("missing Name")
	}
	return nil
}

// Install installs and starts the service.
func (s *Service) Install() error {
	err := s.validate()
	if err != nil {
		return err
	}
	if s.Installed() {
		return errors.New(fmt.Sprintf("Service %s already installed", s.Name))
	}

	logger.Infof("Installing Service %v", s.Name)
	cmd := fmt.Sprintf(serviceInstallScript,
		s.Name,
		s.Conf.Desc,
		s.Conf.Cmd)
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
		Name: name,
		Conf: conf,
	}
}

// InstallCommands returns shell commands to install and start the service.
func (s *Service) InstallCommands() ([]string, error) {
	cmd := fmt.Sprintf(serviceInstallScript,
		s.Name,
		s.Conf.Desc,
		s.Conf.Cmd)
	return strings.Split(cmd, "\n"), nil
}
