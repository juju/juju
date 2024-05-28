// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/juju/juju/internal/packaging/commands"
	"github.com/juju/proxy"
)

const (
	// YumExitCode is used to indicate a retryable failure for APT; DNS, config.
	YumExitCode int = 100
)

// yum is the PackageManager implementations for rpm-based systems.
type yum struct {
	basePackageManager
}

// NewYumPackageManager returns a PackageManager for yum-based systems.
func NewYumPackageManager() PackageManager {
	manager := &yum{
		basePackageManager: basePackageManager{
			cmder:       commands.NewYumPackageCommander(),
			retryPolicy: DefaultRetryPolicy(),
		},
	}
	manager.basePackageManager.retryable = manager
	return manager
}

// Search is defined on the PackageManager interface.
func (yum *yum) Search(pack string) (bool, error) {
	_, code, err := RunCommandWithRetry(yum.cmder.SearchCmd(pack), yum, yum.retryPolicy)

	// yum list package returns 1 when it cannot find the package.
	if code == 1 {
		return false, nil
	}
	return true, err
}

// GetProxySettings is defined on the PackageManager interface.
func (yum *yum) GetProxySettings() (proxy.Settings, error) {
	var res proxy.Settings

	args := strings.Fields(yum.cmder.GetProxyCmd())
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

	for _, match := range strings.Split(string(out), "\n") {
		fields := strings.Split(match, "=")
		if len(fields) != 2 {
			continue
		}

		if strings.HasPrefix(fields[0], "https") {
			res.Https = strings.TrimSpace(fields[1])
		} else if strings.HasPrefix(fields[0], "http") {
			res.Http = strings.TrimSpace(fields[1])
		} else if strings.HasPrefix(fields[0], "ftp") {
			res.Ftp = strings.TrimSpace(fields[1])
		}
	}

	return res, nil
}

func (*yum) IsRetryable(code int, output string) bool {
	return code == YumExitCode
}
