// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package linux

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/sshinit"
	"github.com/juju/juju/environs/manual/common"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/service"
	"github.com/juju/juju/state/multiwatcher"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/shell"
	"github.com/juju/utils/ssh"
)

// InitUbuntuUser adds the ubuntu user if it doesn't
// already exist, updates its ~/.ssh/authorized_keys,
// and enables passwordless sudo for it.
//
// InitUbuntuUser will initially attempt to login as
// the ubuntu user, and verify that passwordless sudo
// is enabled; only if this is false will there be an
// attempt with the specified login.
//
// authorizedKeys may be empty, in which case the file
// will be created and left empty.
//
// stdin and stdout will be used for remote sudo prompts,
// if the ubuntu user must be created/updated.
func InitUbuntuUser(host, login, authorizedKeys string, stdin io.Reader, stdout io.Writer) error {
	logger.Infof("initialising %q, user %q", host, login)

	// To avoid unnecessary prompting for the specified login,
	// initUbuntuUser will first attempt to ssh to the machine
	// as "ubuntu" with password authentication disabled, and
	// ensure that it can use sudo without a password.
	//
	// Note that we explicitly do not allocate a PTY, so we
	// get a failure if sudo prompts.
	cmd := ssh.Command("ubuntu@"+host, []string{"sudo", "-n", "true"}, nil)
	if cmd.Run() == nil {
		logger.Infof("ubuntu user is already initialised")
		return nil
	}

	// Failed to login as ubuntu (or passwordless sudo is not enabled).
	// Use specified login, and execute the initUbuntuScript below.
	if login != "" {
		host = login + "@" + host
	}
	script := fmt.Sprintf(initUbuntuScript, utils.ShQuote(authorizedKeys))
	var options ssh.Options
	options.AllowPasswordAuthentication()
	options.EnablePTY()
	cmd = ssh.Command(host, []string{"sudo", "/bin/bash -c " + utils.ShQuote(script)}, &options)
	var stderr bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stdout = stdout // for sudo prompt
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() != 0 {
			err = fmt.Errorf("%v (%v)", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

const initUbuntuScript = `
set -e
(id ubuntu &> /dev/null) || useradd -m ubuntu -s /bin/bash
umask 0077
temp=$(mktemp)
echo 'ubuntu ALL=(ALL) NOPASSWD:ALL' > $temp
install -m 0440 $temp /etc/sudoers.d/90-juju-ubuntu
rm $temp
su ubuntu -c 'install -D -m 0600 /dev/null ~/.ssh/authorized_keys'
export authorized_keys=%s
if [ ! -z "$authorized_keys" ]; then
    su ubuntu -c 'printf "%%s\n" "$authorized_keys" >> ~/.ssh/authorized_keys'
fi`

// DetectSeriesAndHardwareCharacteristics detects the OS
// series and hardware characteristics of the remote machine
// by connecting to the machine and executing a bash script.
var DetectSeriesAndHardwareCharacteristics = detectSeriesAndHardwareCharacteristics

func detectSeriesAndHardwareCharacteristics(host string) (hc instance.HardwareCharacteristics, series string, err error) {
	logger.Infof("Detecting series and characteristics on %s", host)
	cmd := ssh.Command("ubuntu@"+host, []string{"/bin/bash"}, nil)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = bytes.NewBufferString(detectionScript)
	if err := cmd.Run(); err != nil {
		if stderr.Len() != 0 {
			err = fmt.Errorf("%v (%v)", err, strings.TrimSpace(stderr.String()))
		}
		return hc, "", err
	}
	lines := strings.Split(stdout.String(), "\n")
	series = strings.TrimSpace(lines[0])

	arch := arch.NormaliseArch(lines[1])
	hc.Arch = &arch

	// HardwareCharacteristics wants memory in megabytes,
	// meminfo reports it in kilobytes.
	memkB := strings.Fields(lines[2])[1] // "MemTotal: NNN kB"
	hc.Mem = new(uint64)
	*hc.Mem, err = strconv.ParseUint(memkB, 10, 0)
	*hc.Mem /= 1024

	// For each "physical id", count the number of cores.
	// This way we only count physical cores, not additional
	// logical cores due to hyperthreading.
	recorded := make(map[string]bool)
	var physicalId string
	hc.CpuCores = new(uint64)
	for _, line := range lines[3:] {
		if strings.HasPrefix(line, "physical id") {
			physicalId = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		} else if strings.HasPrefix(line, "cpu cores") {
			var cores uint64
			value := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			if cores, err = strconv.ParseUint(value, 10, 0); err != nil {
				return hc, "", err
			}
			if !recorded[physicalId] {
				*hc.CpuCores += cores
				recorded[physicalId] = true
			}
		}
	}
	if *hc.CpuCores == 0 {
		// In the case of a single-core, non-HT CPU, we'll see no
		// "physical id" or "cpu cores" lines.
		*hc.CpuCores = 1
	}

	// TODO(axw) calculate CpuPower. What algorithm do we use?
	logger.Infof("series: %s, characteristics: %s", series, hc)
	return hc, series, nil
}

// CheckProvisioned checks if any juju init service already
// exist on the host machine.
var CheckProvisioned = checkProvisioned

func checkProvisioned(host string) (bool, error) {
	logger.Infof("Checking if %s is already provisioned", host)

	script := service.ListServicesScript()

	cmd := ssh.Command("ubuntu@"+host, []string{"/bin/bash"}, nil)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = strings.NewReader(script)
	if err := cmd.Run(); err != nil {
		if stderr.Len() != 0 {
			err = fmt.Errorf("%v (%v)", err, strings.TrimSpace(stderr.String()))
		}
		return false, err
	}

	output := strings.TrimSpace(stdout.String())
	provisioned := strings.Contains(output, "juju")
	if provisioned {
		logger.Infof("%s is already provisioned [%q]", host, output)
	} else {
		logger.Infof("%s is not provisioned", host)
	}
	return provisioned, nil
}

// detectionScript is the script to run on the remote machine to
// detect the OS series and hardware characteristics.
const detectionScript = `#!/bin/bash
set -e
lsb_release -cs
uname -m
grep MemTotal /proc/meminfo
cat /proc/cpuinfo`

// gatherMachineParams collects all the information we know about the machine
// we are about to provision. It will SSH into that machine as the ubuntu user.
// The hostname supplied should not include a username.
// If we can, we will reverse lookup the hostname by its IP address, and use
// the DNS resolved name, rather than the name that was supplied
func gatherMachineParams(hostname string) (*params.AddMachineParams, error) {

	// Generate a unique nonce for the machine.
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}

	addr, err := common.HostAddress(hostname)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to compute public address for %q", hostname)
	}

	provisioned, err := checkProvisioned(hostname)
	if err != nil {
		err = fmt.Errorf("error checking if provisioned: %v", err)
		return nil, err
	}
	if provisioned {
		return nil, common.ErrProvisioned
	}

	hc, series, err := DetectSeriesAndHardwareCharacteristics(hostname)
	if err != nil {
		err = fmt.Errorf("error detecting linux hardware characteristics: %v", err)
		return nil, err
	}

	// There will never be a corresponding "instance" that any provider
	// knows about. This is fine, and works well with the provisioner
	// task. The provisioner task will happily remove any and all dead
	// machines from state, but will ignore the associated instance ID
	// if it isn't one that the environment provider knows about.

	instanceId := instance.Id(common.ManualInstancePrefix + hostname)
	nonce := fmt.Sprintf("%s:%s", instanceId, uuid.String())
	machineParams := &params.AddMachineParams{
		Series:                  series,
		HardwareCharacteristics: hc,
		InstanceId:              instanceId,
		Nonce:                   nonce,
		Addrs:                   params.FromNetworkAddresses(addr),
		Jobs:                    []multiwatcher.MachineJob{multiwatcher.JobHostUnits},
	}
	return machineParams, nil
}

func runProvisionScript(script, host string, progressWriter io.Writer) error {
	params := sshinit.ConfigureParams{
		Host:           "ubuntu@" + host,
		ProgressWriter: progressWriter,
	}
	return sshinit.RunConfigureScript(script, params)
}

// Script type implements common.ScriptProvisioner interface
// it is used to make/return the a script that will be used for
// manual provisioning the linux machine.
type Script struct {
	icfg *instancecfg.InstanceConfig
}

// NewScript returns a new instance of Script
func NewScript(icfg *instancecfg.InstanceConfig) *Script {
	return &Script{icfg: icfg}
}

// ProvisioningScript generates a bash script that can be
// executed on a remote host to carry out the cloud-init
// configuration.
func (s *Script) ProvisioningScript() (string, error) {
	cloudcfg, err := cloudinit.New(s.icfg.Series)
	if err != nil {
		return "", errors.Annotate(err, "error generating cloud-config")
	}
	cloudcfg.SetSystemUpdate(s.icfg.EnableOSRefreshUpdate)
	cloudcfg.SetSystemUpgrade(s.icfg.EnableOSUpgrade)

	udata, err := cloudconfig.NewUserdataConfig(s.icfg, cloudcfg)
	if err != nil {
		return "", errors.Annotate(err, "error generating cloud-config")
	}
	if err := udata.ConfigureJuju(); err != nil {
		return "", errors.Annotate(err, "error generating cloud-config")
	}

	configScript, err := cloudcfg.RenderScript()
	if err != nil {
		return "", errors.Annotate(err, "error converting cloud-config to script")
	}

	var buf bytes.Buffer
	// Always remove the cloud-init-output.log file first, if it exists.
	fmt.Fprintf(&buf, "rm -f %s\n", utils.ShQuote(s.icfg.CloudInitOutputLog))
	// If something goes wrong, dump cloud-init-output.log to stderr.
	buf.WriteString(shell.DumpFileOnErrorScript(s.icfg.CloudInitOutputLog))
	buf.WriteString(configScript)
	return buf.String(), nil
}