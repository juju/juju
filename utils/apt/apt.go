// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apt

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/proxy"
)

var (
	logger  = loggo.GetLogger("juju.utils.apt")
	proxyRE = regexp.MustCompile(`(?im)^\s*Acquire::(?P<protocol>[a-z]+)::Proxy\s+"(?P<proxy>[^"]+)";\s*$`)

	// ConfFile is the full file path for the proxy settings that are
	// written by cloud-init and the machine environ worker.
	ConfFile = "/etc/apt/apt.conf.d/42-juju-proxy-settings"
)

// Some helpful functions for running apt in a sane way

// CommandOutput calls cmd.Output, this is used as an overloading point so we
// can test what *would* be run without actually executing another program
var CommandOutput = (*exec.Cmd).CombinedOutput

// getCommand is the default apt-get command used in cloud-init, the various settings
// mean that apt won't actually block waiting for a prompt from the user.
var getCommand = []string{
	"apt-get", "--option=Dpkg::Options::=--force-confold",
	"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
}

// getEnvOptions are options we need to pass to apt-get to not have it prompt
// the user
var getEnvOptions = []string{"DEBIAN_FRONTEND=noninteractive"}

// cloudArchivePackages maintaines a list of packages that GetPreparePackages
// should reference when determining the --target-release for a given series.
// http://reqorts.qa.ubuntu.com/reports/ubuntu-server/cloud-archive/cloud-tools_versions.html
var cloudArchivePackages = map[string]bool{
	"cloud-image-utils":       true,
	"cloud-utils":             true,
	"curtin":                  true,
	"djorm-ext-pgarray":       true,
	"golang":                  true,
	"iproute2":                true,
	"isc-dhcp":                true,
	"juju-core":               true,
	"libseccomp":              true,
	"libv8-3.14":              true,
	"lxc":                     true,
	"maas":                    true,
	"mongodb":                 true,
	"mongodb-server":          true,
	"python-django":           true,
	"python-django-piston":    true,
	"python-jujuclient":       true,
	"python-tx-tftp":          true,
	"python-websocket-client": true,
	"raphael 2.1.0-1ubuntu1":  true,
	"simplestreams":           true,
	"txlongpoll":              true,
	"uvtool":                  true,
	"yui3":                    true,
}

// targetRelease returns a string base on the current series
// that is suitable for use with the apt-get --target-release option
func targetRelease(series string) string {
	switch series {
	case "precise":
		return "precise-updates/cloud-tools"
	default:
		return ""
	}
}

// GetPreparePackages returns a slice of installCommands. Each item
// in the slice is suitable for passing directly to Apt.
// It inspects the series passed to it
// and properly generates an installCommand entry with a --target-release
// should the series be an LTS release with cloud archive packages.
func GetPreparePackages(packages []string, series string) [][]string {
	var installCommands [][]string
	if target := targetRelease(series); target == "" {
		return append(installCommands, packages)
	} else {
		var pkgs []string
		pkgs_with_target := []string{"--target-release", target}
		for _, pkg := range packages {
			if cloudArchivePackages[pkg] {
				pkgs_with_target = append(pkgs_with_target, pkg)
			} else {
				pkgs = append(pkgs, pkg)
			}
		}

		// We check for >2 here so that we only append pkgs_with_target
		// if there was an actual package in the slice.
		if len(pkgs_with_target) > 2 {
			installCommands = append(installCommands, pkgs_with_target)
		}

		// Sometimes we may end up with all cloudArchivePackages
		// in that case we do not want to append an empty slice of pkgs
		if len(pkgs) > 0 {
			installCommands = append(installCommands, pkgs)
		}

		return installCommands
	}
}

// GetInstall runs 'apt-get install packages' for the packages listed here
func GetInstall(packages ...string) error {
	cmdArgs := append([]string(nil), getCommand...)
	cmdArgs = append(cmdArgs, "install")
	cmdArgs = append(cmdArgs, packages...)
	logger.Infof("Running: %s", cmdArgs)
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = append(os.Environ(), getEnvOptions...)
	out, err := CommandOutput(cmd)
	if err != nil {
		logger.Errorf("apt-get command failed: %v\nargs: %#v\n%s",
			err, cmdArgs, string(out))
		return fmt.Errorf("apt-get failed: %v", err)
	}
	return nil
}

// ConfigProxy will consult apt-config about the configured proxy
// settings. If there are no proxy settings configured, an empty string is
// returned.
func ConfigProxy() (string, error) {
	cmdArgs := []string{
		"apt-config",
		"dump",
		"Acquire::http::Proxy",
		"Acquire::https::Proxy",
		"Acquire::ftp::Proxy",
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	out, err := CommandOutput(cmd)
	if err != nil {
		logger.Errorf("apt-config command failed: %v\nargs: %#v\n%s",
			err, cmdArgs, string(out))
		return "", fmt.Errorf("apt-config failed: %v", err)
	}
	return string(bytes.Join(proxyRE.FindAll(out, -1), []byte("\n"))), nil
}

// DetectProxies will parse the results of ConfigProxy to return a
// ProxySettings instance.
func DetectProxies() (result proxy.Settings, err error) {
	output, err := ConfigProxy()
	if err != nil {
		return result, err
	}
	for _, match := range proxyRE.FindAllStringSubmatch(output, -1) {
		switch match[1] {
		case "http":
			result.Http = match[2]
		case "https":
			result.Https = match[2]
		case "ftp":
			result.Ftp = match[2]
		}
	}
	return result, nil
}

// ProxyContent produces the format expected by the apt config files
// from the ProxySettings struct.
func ProxyContent(proxySettings proxy.Settings) string {
	lines := []string{}
	addLine := func(proxySettings, value string) {
		if value != "" {
			lines = append(lines, fmt.Sprintf(
				"Acquire::%s::Proxy %q;", proxySettings, value))
		}
	}
	addLine("http", proxySettings.Http)
	addLine("https", proxySettings.Https)
	addLine("ftp", proxySettings.Ftp)
	return strings.Join(lines, "\n")
}

// IsPackageInstalled uses dpkg-query to determine if the `packageName`
// package is installed.
func IsPackageInstalled(packageName string) bool {
	_, err := utils.RunCommand("dpkg-query", "--status", packageName)
	return err == nil
}
