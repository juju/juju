// Copyright 2014 Cloudbase Solutions
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/service/initsystems"
)

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

func runPsCommand(cmd string) (*exec.ExecResponse, error) {
	com := exec.RunParams{
		Commands: cmd,
	}
	out, err := exec.RunCommands(com)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if out.Code != 0 {
		return nil, errors.Errorf("running %s: %s", cmd, string(out.Stderr))
	}
	return out, nil
}

type windows struct {
	name string
}

func NewInitSystem(name string) initsystems.InitSystem {
	return &windows{
		name: name,
	}
}

func (is *windows) Name() string {
	return is.name
}

func (is *windows) List(include ...string) ([]string, error) {
	out, err := runPsCommand(`(Get-Service).Name`)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return strings.Fields(string(out.Stdout)), nil
}

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
	_, err = runPsCommand(cmd)
	return err
}

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
	_, err = runPsCommand(cmd)
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
		_, err := runPsCommand(command)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
	/*
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
		outCmd, errCmd := runPsCommand(cmd)

		if errCmd != nil {
			logger.Infof("ERROR installing service %v --> %v", outCmd, errCmd)
			return errCmd
		}
		return s.Start()
	*/
}

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
	_, err = runPsCommand(cmd)
	return err
}

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
	out, err := runPsCommand(cmd)
	if err != nil {
		return "", errors.Trace(err)
	}

	status := initsystems.StatusRunning
	if strings.TrimSpace(string(out.Stdout)) == "Stopped" {
		status = initsystems.StatusStopped
	}
	return status, nil
}

func (is *windows) Conf(name string) (*initsystems.Conf, error) {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(ericsnow) Finish!
	return nil, nil
}

func (is *windows) Validate(name string, conf initsystems.Conf) error {
	err := Validate(name, conf)
	return errors.Trace(err)
}

func (is *windows) Serialize(name string, conf initsystems.Conf) ([]byte, error) {
	data, err := Serialize(name, conf)
	return data, errors.Trace(err)
}

func (is *windows) Deserialize(data []byte) (*initsystems.Conf, error) {
	conf, err := Deserialize(data)
	return conf, errors.Trace(err)
}
