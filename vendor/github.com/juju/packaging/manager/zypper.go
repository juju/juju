// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/juju/proxy"
)

// yum is the PackageManager implementations for rpm-based systems.
type zypper struct {
	basePackageManager
}

// Search is defined on the PackageManager interface.
func (zypper *zypper) Search(pack string) (bool, error) {
	_, code, err := RunCommandWithRetry(zypper.cmder.SearchCmd(pack), nil)

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

	output := string(bytes.Join(proxyRE.FindAll(out, -1), []byte("\n")))

	for _, match := range proxyRE.FindAllStringSubmatch(output, -1) {
		switch match[1] {
		case "http":
			res.Http = match[2]
		case "https":
			res.Https = match[2]
		case "ftp":
			res.Ftp = match[2]
		}
	}

	return res, nil
}
