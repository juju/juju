// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/packaging/commands"
)

const (
	// APTExitCode is used to indicate a retryable failure for APT; DNS, config.
	APTExitCode int = 100
)

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

// Install is defined on the PackageManager interface.
func (apt *apt) Install(packs ...string) error {
	_, _, err := RunCommandWithRetry(apt.cmder.InstallCmd(packs...), apt.installRetryable, apt.retryPolicy)
	return err
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
