// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/juju/juju/internal/packaging/commands"
	"github.com/juju/proxy"
)

// zypperProxyRE is a regexp which matches all proxy-related configuration options in
// the apt configuration file.
var zypperProxyRE = regexp.MustCompile(`(?im)^\s*(?P<protocol>[a-z\_]+)\s*=\s*(?P<proxy>.*)\s*$`)

// zypper is the PackageManager implementations for openSUSE systems.
type zypper struct {
	basePackageManager
}

func NewZypperPackageManager() PackageManager {
	// Note: this does not override the default retrier, as Zypper doesn't have
	// any retryable CLI command codes.
	return &zypper{
		basePackageManager{
			cmder:       commands.NewZypperPackageCommander(),
			retryPolicy: DefaultRetryPolicy(),
		},
	}
}

// Search is defined on the PackageManager interface.
func (zypper *zypper) Search(pack string) (bool, error) {
	_, code, err := RunCommandWithRetry(zypper.cmder.SearchCmd(pack), zypper, zypper.retryPolicy)

	// zypper search returns 104 when it cannot find the package.
	if code == 104 {
		return false, nil
	}
	return true, err
}

// GetProxySettings is defined on the PackageManager interface.
func (zypper *zypper) GetProxySettings() (proxy.Settings, error) {
	var res proxy.Settings

	args := strings.Fields(zypper.cmder.GetProxyCmd())
	if len(args) <= 1 {
		return proxy.Settings{}, fmt.Errorf("expected at least 2 arguments, got %d %v", len(args), args)
	}

	cmd := exec.Command(args[0], args[1:]...)
	out, err := CommandOutput(cmd)

	if err != nil {
		logger.Errorf("command failed: %v\nargs: %#v\n%s",
			err, args, string(out))
		return res, fmt.Errorf("command failed: %v", err)
	}

	output := string(bytes.Join(zypperProxyRE.FindAll(out, -1), []byte("\n")))
	for _, match := range zypperProxyRE.FindAllStringSubmatch(output, -1) {
		switch strings.ToLower(match[1]) {
		case "http_proxy":
			res.Http = match[2]
		case "https_proxy":
			res.Https = match[2]
		case "ftp_proxy":
			res.Ftp = match[2]
		}
	}

	return res, nil
}
