// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/shell"
	"github.com/juju/utils/ssh"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/sshinit"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/service"
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
func InitUbuntuUser(host, login, authorizedKeys string, read io.Reader, write io.Writer) error {
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
	cmd.Stdin = read
	cmd.Stdout = write
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
(grep ubuntu /etc/group) || groupadd ubuntu
(id ubuntu &> /dev/null) || useradd -m ubuntu -s /bin/bash -g ubuntu
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
	var processorEntries uint64
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
		} else if strings.HasPrefix(line, "processor") {
			processorEntries++
		}
	}
	if *hc.CpuCores == 0 {
		// As a fallback, if there're no `physical id` entries, we count `processor` entries
		// This happens on arm, arm64, ppc, see lp:1664434
		*hc.CpuCores = processorEntries
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
	provisioned := strings.Contains(output, "jujud-machine")
	return provisioned, nil
}

// detectionScript is the script to run on the remote machine to
// detect the OS series and hardware characteristics.
const detectionScript = `#!/bin/bash
set -e
os_id=$(grep '^ID=' /etc/os-release | tr -d '"' | cut -d= -f2)
if [ "$os_id" = 'centos' ]; then
  os_version=$(grep '^VERSION_ID=' /etc/os-release | tr -d '"' | cut -d= -f2)
  echo "centos$os_version"
elif [ "$os_id" = 'opensuse' ]; then
  os_version=$(grep '^VERSION_ID=' /etc/os-release | tr -d '"' | cut -d= -f2 | cut -d. -f1)
  if [ $os_version -eq 42 ]; then
    echo "opensuseleap"
  fi
else
  lsb_release -cs
fi
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

	addr, err := manual.HostAddress(hostname)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to compute public address for %q", hostname)
	}

	provisioned, err := checkProvisioned(hostname)
	if err != nil {
		return nil, errors.Annotatef(err, "error checking if provisioned")
	}
	if provisioned {
		return nil, manual.ErrProvisioned
	}

	hc, series, err := DetectSeriesAndHardwareCharacteristics(hostname)
	if err != nil {
		return nil, errors.Annotatef(err, "error detecting linux hardware characteristics")
	}

	// There will never be a corresponding "instance" that any provider
	// knows about. This is fine, and works well with the provisioner
	// task. The provisioner task will happily remove any and all dead
	// machines from state, but will ignore the associated instance ID
	// if it isn't one that the environment provider knows about.
	instanceId := instance.Id(manual.ManualInstancePrefix + hostname)
	nonce := fmt.Sprintf("%s:%s", instanceId, uuid.String())
	machineParams := &params.AddMachineParams{
		Series:                  series,
		HardwareCharacteristics: hc,
		InstanceId:              instanceId,
		Nonce:                   nonce,
		Addrs:                   params.FromProviderAddresses(addr),
		Jobs:                    []model.MachineJob{model.JobHostUnits},
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

// ProvisioningScript generates a bash script that can be
// executed on a remote host to carry out the cloud-init
// configuration.
func ProvisioningScript(icfg *instancecfg.InstanceConfig) (string, error) {
	cloudcfg, err := cloudinit.New(icfg.Series)
	if err != nil {
		return "", errors.Annotate(err, "error generating cloud-config")
	}
	cloudcfg.SetSystemUpdate(icfg.EnableOSRefreshUpdate)
	cloudcfg.SetSystemUpgrade(icfg.EnableOSUpgrade)

	udata, err := cloudconfig.NewUserdataConfig(icfg, cloudcfg)
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
	fmt.Fprintf(&buf, "rm -f %s\n", utils.ShQuote(icfg.CloudInitOutputLog))
	// If something goes wrong, dump cloud-init-output.log to stderr.
	buf.WriteString(shell.DumpFileOnErrorScript(icfg.CloudInitOutputLog))
	buf.WriteString(configScript)
	return buf.String(), nil
}
