// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"launchpad.net/juju-core/log"
	"launchpad.net/loggo"
)

var (
	aptLogger  = loggo.GetLogger("juju.utils.apt")
	aptProxyRE = regexp.MustCompile(`(?i)^Acquire::[a-z]+::Proxy\s+"([^"]+)";$`)
)

// Some helpful functions for running apt in a sane way

// commandOutput calls cmd.Output, this is used as an overloading point so we
// can test what *would* be run without actually executing another program
var commandOutput = (*exec.Cmd).CombinedOutput

// This is the default apt-get command used in cloud-init, the various settings
// mean that apt won't actually block waiting for a prompt from the user.
var aptGetCommand = []string{
	"apt-get", "--option=Dpkg::Options::=--force-confold",
	"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
}

// aptEnvOptions are options we need to pass to apt-get to not have it prompt
// the user
var aptGetEnvOptions = []string{"DEBIAN_FRONTEND=noninteractive"}

// AptGetInstall runs 'apt-get install packages' for the packages listed here
func AptGetInstall(packages ...string) error {
	cmdArgs := append([]string(nil), aptGetCommand...)
	cmdArgs = append(cmdArgs, "install")
	cmdArgs = append(cmdArgs, packages...)
	aptLogger.Infof("Running: %s", cmdArgs)
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = append(os.Environ(), aptGetEnvOptions...)
	out, err := commandOutput(cmd)
	if err != nil {
		log.Errorf("utils/apt: apt-get command failed: %v\nargs: %#v\n%s",
			err, cmdArgs, string(out))
		return fmt.Errorf("apt-get failed: %v", err)
	}
	return nil
}

// AptConfigProxy will consult apt-config about the configured proxy
// settings. If there are no proxy settings configured, an empty string is
// returned.
func AptConfigProxy() (string, error) {
	cmdArgs := []string{
		"apt-config",
		"dump",
		"Acquire::http::Proxy",
		"Acquire::https::Proxy",
		"Acquire::ftp::Proxy",
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	out, err := commandOutput(cmd)
	if err != nil {
		log.Errorf("utils/apt: apt-config command failed: %v\nargs: %#v\n%s",
			err, cmdArgs, string(out))
		return "", fmt.Errorf("apt-config failed: %v", err)
	}
	proxyConfig := strings.TrimSpace(string(out))
	if proxyConfig != "" {
		var proxyLines []string
		for _, line := range strings.Split(proxyConfig, "\n") {
			line = strings.TrimSpace(line)
			if m := aptProxyRE.FindStringSubmatch(line); m != nil {
				proxyLines = append(proxyLines, line)
			}
		}
		if len(proxyLines) > 0 {
			proxyConfig = strings.Join(proxyLines, "\n")
		}
	}
	return proxyConfig, nil
}
