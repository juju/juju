// Copyright 2014 Cloudbase Solutions
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/service/initsystems"
)

type windows struct {
	name string
}

// NewInitSystem returns a new value that implements
// initsystems.InitSystem for Windows.
func NewInitSystem(name string) initsystems.InitSystem {
	return &windows{
		name: name,
	}
}

// Name implements service/initsystems.InitSystem.
func (is *windows) Name() string {
	return is.name
}

// List implements service/initsystems.InitSystem.
func (is *windows) List(include ...string) ([]string, error) {
	out, err := initsystems.RunPsCommand(`(Get-Service).Name`)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return strings.Fields(string(out.Stdout)), nil
}

// Start implements service/initsystems.InitSystem.
func (is *windows) Start(name string) error {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	// Fail if already running.
	status, err := is.status(name)
	if err != nil {
		return errors.Trace(err)
	}
	if status == initsystems.StatusRunning {
		return errors.AlreadyExistsf("service %q", name)
	}

	// Send the start request.
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; Start-Service  "%s"`, name)
	_, err = initsystems.RunPsCommand(cmd)
	return err
}

// Stop implements service/initsystems.InitSystem.
func (is *windows) Stop(name string) error {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	// Fail if not running.
	status, err := is.status(name)
	if err != nil {
		return errors.Trace(err)
	}
	if status != initsystems.StatusRunning {
		return errors.NotFoundf("service %q", name)
	}

	// Send the stop request.
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; Stop-Service  "%s"`, name)
	_, err = initsystems.RunPsCommand(cmd)
	return err
}

func (is *windows) readConf(filename string) (*initsystems.Conf, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.Trace(err)
	}
	conf, err := is.Deserialize(data)
	return conf, errors.Trace(err)
}

// Enable implements service/initsystems.InitSystem.
func (is *windows) Enable(name, filename string) error {
	// TODO(ericsnow) Finish!
	return nil

	conf, err := is.readConf(filename)
	if err != nil {
		return errors.Trace(err)
	}

	if err := Validate(name, *conf); err != nil {
		return errors.Trace(err)
	}

	// (from environs/cloudinit/cloudinit_win.go)
	commands := []string{
		fmt.Sprintf(`New-Service -Credential $jujuCreds -Name '%s' -DisplayName '%s' '%s'`, name, conf.Desc, conf.Cmd),
		fmt.Sprintf(`cmd.exe /C sc config %s start=delayed-auto`, name),
		fmt.Sprintf(`Start-Service %s`, name),
	}

	for _, command := range commands {
		_, err := initsystems.RunPsCommand(command)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
	/*
		        // From the old Install method.
				err := Validate(s.Name, s.Conf)
				if err != nil {
					return errors.Trace(err)
				}
				if s.Installed() {
					return errors.New(fmt.Sprintf("Service %s already installed", s.Name))
				}

				logger.Infof("Installing Service %v", s.Name)
				cmd := fmt.Sprintf(serviceInstallScript,
					s.Name,
					s.Conf.Desc,
					s.Conf.Cmd)
				outCmd, errCmd := initsystems.RunPsCommand(cmd)

				if errCmd != nil {
					logger.Infof("ERROR installing service %v --> %v", outCmd, errCmd)
					return errCmd
				}
				return s.Start()
	*/
}

// Disable implements service/initsystems.InitSystem.
func (is *windows) Disable(name string) error {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) Finish!
	return nil
	_, err := is.status(name)
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; (gwmi win32_service -filter 'name="%s"').Delete()`, name)
	_, err = initsystems.RunPsCommand(cmd)
	return err
}

// IsEnabled implements service/initsystems.InitSystem.
func (is *windows) IsEnabled(name string) (bool, error) {
	_, err := is.status(name)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
}

func isNotFound(err error) bool {
	// TODO(ericsnow) Check for a specific error (or error message).
	return true
}

// Info implements service/initsystems.InitSystem.
func (is *windows) Info(name string) (*initsystems.ServiceInfo, error) {
	status, err := is.status(name)
	if isNotFound(err) {
		return nil, errors.NotFoundf("service %q", name)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	info := &initsystems.ServiceInfo{
		Name: name,
		// Desc
		Status: status,
	}
	return info, nil
}

func (is *windows) status(name string) (string, error) {
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; (Get-Service "%s").Status`, name)
	out, err := initsystems.RunPsCommand(cmd)
	if err != nil {
		return "", errors.Trace(err)
	}

	status := initsystems.StatusRunning
	if strings.TrimSpace(string(out.Stdout)) == "Stopped" {
		status = initsystems.StatusStopped
	}
	return status, nil
}

// Conf implements service/initsystems.InitSystem.
func (is *windows) Conf(name string) (*initsystems.Conf, error) {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(ericsnow) Finish!
	return nil, nil
}

// Validate implements service/initsystems.InitSystem.
func (is *windows) Validate(name string, conf initsystems.Conf) error {
	err := Validate(name, conf)
	return errors.Trace(err)
}

// Serialize implements service/initsystems.InitSystem.
func (is *windows) Serialize(name string, conf initsystems.Conf) ([]byte, error) {
	data, err := Serialize(name, conf)
	return data, errors.Trace(err)
}

// Deserialize implements service/initsystems.InitSystem.
func (is *windows) Deserialize(data []byte) (*initsystems.Conf, error) {
	conf, err := Deserialize(data)
	return conf, errors.Trace(err)
}

// for cloud-init:
func installCommands(name string, conf initsystems.Conf) ([]string, error) {
	cmd := fmt.Sprintf(serviceInstallScript,
		name,
		conf.Desc,
		conf.Cmd)
	return strings.Split(cmd, "\n"), nil
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
