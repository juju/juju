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

	"github.com/juju/juju/service/common"
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

type initSystem struct {
	name string
}

func NewInitSystem(name string) common.InitSystem {
	return &initSystem{
		name: name,
	}
}

func (is *initSystem) Name() string {
	return is.name
}

func (is *initSystem) List(include ...string) ([]string, error) {
	out, err := runPsCommand(`(Get-Service).Name`)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return strings.Fields(string(out.Stdout)), nil
}

func (is *initSystem) Start(name string) error {
	// TODO(ericsnow) Finish!
	return nil
}

func (is *initSystem) Stop(name string) error {
	// TODO(ericsnow) Finish!
	return nil
}

func (is *initSystem) readConf(filename string) (*common.Conf, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.Trace(err)
	}
	conf, err := is.Deserialize(data)
	return conf, errors.Trace(err)
}

func (is *initSystem) Enable(name, filename string) error {
	conf, err := is.readConf(filename)
	if err != nil {
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
}

func (is *initSystem) Disable(name string) error {
	// TODO(ericsnow) Finish!
	return nil
}

func (is *initSystem) IsEnabled(name string) (bool, error) {
	// TODO(ericsnow) Finish!
	return false, nil
}

func (is *initSystem) Info(name string) (*common.ServiceInfo, error) {
	// TODO(ericsnow) Finish!
	return nil, nil
}

func (is *initSystem) Conf(name string) (*common.Conf, error) {
	// TODO(ericsnow) Finish!
	return nil, nil
}

func (is *initSystem) Serialize(name string, conf common.Conf) ([]byte, error) {
	// TODO(ericsnow) Finish!
	return nil, nil
}

func (is *initSystem) Deserialize(data []byte) (*common.Conf, error) {
	// TODO(ericsnow) Finish!
	return nil, nil
}
