// Copyright 2014 Cloudbase Solutions
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package windows

import (
	"fmt"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/service/initsystems"
)

// TODO(ericsnow) Remove juju-specific pieces.

type windows struct {
	name string
	fops fileOperations
	cmd  cmdRunner
}

// NewInitSystem returns a new value that implements
// initsystems.InitSystem for Windows.
func NewInitSystem(name string) initsystems.InitSystem {
	return &windows{
		name: name,
		fops: newFileOperations(),
		cmd:  newCmdRunner(),
	}
}

// Name implements service/initsystems.InitSystem.
func (is *windows) Name() string {
	return is.name
}

// List implements service/initsystems.InitSystem.
func (is *windows) List(include ...string) ([]string, error) {
	out, err := is.cmd.RunCommandStr(`(Get-Service).Name`)
	if err != nil {
		return nil, errors.Trace(err)
	}
	services := strings.Fields(string(out))
	return initsystems.FilterNames(services, include), nil
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
	_, err = is.cmd.RunCommandStr(cmd)
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
	_, err = is.cmd.RunCommandStr(cmd)
	return err
}

// Enable implements service/initsystems.InitSystem.
func (is *windows) Enable(name, filename string) error {
	enabled, err := is.IsEnabled(name)
	if err != nil {
		return errors.Trace(err)
	}
	if enabled {
		return errors.AlreadyExistsf("service %q", name)
	}

	data, err := is.fops.ReadFile(filename)
	if err != nil {
		return errors.Trace(err)
	}
	conf, err := is.Deserialize(data)
	if err != nil {
		return errors.Trace(err)
	}

	commands := enableCommands(name, *conf)
	for _, command := range commands {
		_, err := is.cmd.RunCommandStr(command)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// Disable implements service/initsystems.InitSystem.
func (is *windows) Disable(name string) error {
	if err := initsystems.EnsureEnabled(name, is); err != nil {
		return errors.Trace(err)
	}

	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; (gwmi win32_service -filter 'name="%s"').Delete()`, name)
	_, err := is.cmd.RunCommandStr(cmd)
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
	if err == nil {
		return false
	}

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

	// TODO(ericsnow) Pull the description from somewhere?

	info := &initsystems.ServiceInfo{
		Name: name,
		// Description
		Status: status,
	}
	return info, nil
}

func (is *windows) status(name string) (string, error) {
	cmd := fmt.Sprintf(`$ErrorActionPreference="Stop"; (Get-Service "%s").Status`, name)
	out, err := is.cmd.RunCommandStr(cmd)
	if err != nil {
		return "", errors.Trace(err)
	}

	var status string
	switch strings.TrimSpace(string(out)) {
	case "Stopped":
		status = initsystems.StatusStopped
	default:
		// TODO(ericsnow) Fail here and handle "Running" in a case.
		status = initsystems.StatusRunning

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
func installCommands(name string, conf initsystems.Conf) []string {
	cmds := enableCommands(name, conf)
	cmds = append(cmds,
		// (from environs/cloudinit/cloudinit_win.go)
		fmt.Sprintf(`Start-Service %s`, name),
	)
	return cmds
}

func enableCommands(name string, conf initsystems.Conf) []string {
	// (from environs/cloudinit/cloudinit_win.go)
	return []string{
		fmt.Sprintf(`New-Service -Credential $jujuCreds -Name '%s' -DisplayName '%s' '%s'`, name, conf.Desc, conf.Cmd),
		fmt.Sprintf(`cmd.exe /C sc config %s start=delayed-auto`, name),
	}

	// TODO(ericsnow) Use the full install script (from
	// service/windows/service.go)?
	cmd := fmt.Sprintf(serviceInstallScript,
		name,
		conf.Desc,
		conf.Cmd)
	return strings.Split(cmd, "\n")
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
