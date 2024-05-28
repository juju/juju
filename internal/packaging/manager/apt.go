// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/internal/packaging/commands"
	"github.com/juju/proxy"
)

const (
	// APTExitCode is used to indicate a retryable failure for APT; DNS, config.
	APTExitCode int = 100
)

// aptProxyRE is a regexp which matches all proxy-related configuration options in
// the apt configuration file.
var aptProxyRE = regexp.MustCompile(`(?im)^\s*Acquire::(?P<protocol>[a-z]+)::Proxy\s+"(?P<proxy>[^"]+)";\s*$`)

// apt is the PackageManager implementation for deb-based systems.
type apt struct {
	basePackageManager
	installRetryable Retryable
}

// NewAptPackageManager returns a PackageManager for apt-based systems.
func NewAptPackageManager() PackageManager {
	manager := &apt{
		basePackageManager: basePackageManager{
			cmder:       commands.NewAptPackageCommander(),
			retryPolicy: DefaultRetryPolicy(),
		},
		installRetryable: makeAPTInstallRetryable(APTExitCode),
	}
	// Push the base retryable on the type itself as that has the context
	// to make the choices.
	manager.basePackageManager.retryable = manager
	return manager
}

// Search is defined on the PackageManager interface.
func (apt *apt) Search(pack string) (bool, error) {
	out, _, err := RunCommandWithRetry(apt.cmder.SearchCmd(pack), apt, apt.retryPolicy)
	if err != nil {
		return false, err
	}

	// apt-cache search --names-only package returns no output
	// if the search was unsuccessful
	if out == "" {
		return false, nil
	}
	return true, nil
}

// Install is defined on the PackageManager interface.
func (apt *apt) Install(packs ...string) error {
	_, _, err := RunCommandWithRetry(apt.cmder.InstallCmd(packs...), apt.installRetryable, apt.retryPolicy)
	return err
}

// GetProxySettings is defined on the PackageManager interface.
func (apt *apt) GetProxySettings() (proxy.Settings, error) {
	var res proxy.Settings

	args := strings.Fields(apt.cmder.GetProxyCmd())
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

	output := string(bytes.Join(aptProxyRE.FindAll(out, -1), []byte("\n")))

	for _, match := range aptProxyRE.FindAllStringSubmatch(output, -1) {
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

func (*apt) IsRetryable(code int, output string) bool {
	return code == APTExitCode
}

// aptInstallRetryable defines a retryable that focuses on apt install process.
// All other apt retryables are done via the base manager.
type aptInstallRetryable struct {
	exitCode int
}

func makeAPTInstallRetryable(exitCode int) aptInstallRetryable {
	return aptInstallRetryable{
		exitCode: exitCode,
	}
}

// IsRetryable returns if the following is retryable from looking at the
// cmd exit code and the stdout/stderr output.
func (r aptInstallRetryable) IsRetryable(code int, output string) bool {
	if code != r.exitCode {
		return false
	}
	if r.isFatalError(output) {
		return false
	}
	return true
}

// MaskError will mask an error using the cmd exit code and the stdout/stderr
// output.
func (r aptInstallRetryable) MaskError(code int, output string) error {
	if r.isFatalError(output) {
		return errors.New("unable to locate package")
	}
	return nil
}

func (r aptInstallRetryable) isFatalError(output string) bool {
	// If we couldn't find the package don't retry.
	// apt-get will report "Unable to locate package"
	return strings.Contains(output, "Unable to locate package")
}
