// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
)

// checkProvisionedScript is the script to run on the remote machine
// to check if a machine has already been provisioned.
//
// This is a little convoluted to avoid returning an error in the
// common case of no matching files.
const checkProvisionedScript = "ls /etc/init/ | grep juju.*\\.conf || exit 0"

// checkProvisioned checks if any juju upstart jobs already
// exist on the host machine.
func checkProvisioned(sshHost string) (bool, error) {
	logger.Infof("Checking if %s is already provisioned", sshHost)
	cmd := ssh.Command(sshHost, fmt.Sprintf("bash -c %s", utils.ShQuote(checkProvisionedScript)))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() != 0 {
			err = fmt.Errorf("%v (%v)", err, strings.TrimSpace(stderr.String()))
		}
		return false, err
	}
	output := strings.TrimSpace(stdout.String())
	provisioned := len(output) > 0
	if provisioned {
		logger.Infof("%s is already provisioned [%q]", sshHost, output)
	} else {
		logger.Infof("%s is not provisioned", sshHost)
	}
	return provisioned, nil
}

// DetectSeriesAndHardwareCharacteristics detects the OS
// series and hardware characteristics of the remote machine
// by connecting to the machine and executing a bash script.
//
// The sshHost argument must be a hostname of the form [user@]host.
func DetectSeriesAndHardwareCharacteristics(sshHost string) (hc instance.HardwareCharacteristics, series string, err error) {
	logger.Infof("Detecting series and characteristics on %s", sshHost)
	cmd := ssh.Command(sshHost, "bash")
	cmd.Stdin = bytes.NewBufferString(detectionScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) != 0 {
			err = fmt.Errorf("%v (%v)", err, strings.TrimSpace(string(out)))
		}
		return hc, "", err
	}
	lines := strings.Split(string(out), "\n")
	series = strings.TrimSpace(lines[0])

	// Normalise arch.
	arch := strings.TrimSpace(lines[1])
	for _, re := range archREs {
		if re.Match([]byte(arch)) {
			hc.Arch = &re.arch
			break
		}
	}
	if hc.Arch == nil {
		err = fmt.Errorf("unrecognised architecture: %s", arch)
		return hc, "", err
	}

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

// archREs maps regular expressions for matching
// `uname -m` to architectures recognised by Juju.
var archREs = []struct {
	*regexp.Regexp
	arch string
}{
	{regexp.MustCompile("amd64|x86_64"), "amd64"},
	{regexp.MustCompile("i[3-9]86"), "i386"},
	{regexp.MustCompile("armv.*"), "arm"},
}

const detectionScript = `#!/bin/bash
set -e
lsb_release -cs
uname -m
grep MemTotal /proc/meminfo
cat /proc/cpuinfo`
