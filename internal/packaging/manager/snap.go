// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/internal/packaging/commands"
)

var (
	// SnapExitCodes is used to indicate a retryable failure for Snap.
	// See list of failures.
	//
	// Test the following exit codes. 1 and 2 are failures and depending on the
	// error message can be retried. There is a exit code of 10, which can be
	// blindly retried, but that's not implemented yet.
	SnapExitCodes = []int{1, 2}

	// SnapAttempts describe the number of attempts to retry each command.
	SnapAttempts = 3

	snapNotFoundRE = regexp.MustCompile(`(?i)error: snap "[^"]+" not found`)
	trackingRE     = regexp.MustCompile(`(?im)tracking:\s*(.*)$`)

	_ PackageManager = (*Snap)(nil)
)

// Snap is the PackageManager implementation for snap-based systems.
type Snap struct {
	basePackageManager
	installRetryable Retryable
}

// NewSnapPackageManager returns a PackageManager for snap-based systems.
func NewSnapPackageManager() *Snap {
	return &Snap{
		basePackageManager: basePackageManager{
			cmder: commands.NewSnapPackageCommander(),
			retryPolicy: RetryPolicy{
				Delay:    Delay,
				Attempts: SnapAttempts,
			},
		},
		// InstallRetryable checks a series of strings, to pattern
		// match against the cmd output to see if an install command is
		// retryable.
		installRetryable: makeRegexpRetryable(SnapExitCodes,
			"(?i)mount snap .*? failed",
			"(?i)setup snap .*? security profiles \\(cannot reload udev rules",
		),
	}
}

// InstalledChannel returns the snap channel for an installed package.
func (snap *Snap) InstalledChannel(pack string) string {
	out, _, err := RunCommandWithRetry(fmt.Sprintf("snap info %s", pack), snap, snap.retryPolicy)
	combined := combinedOutput(out, err)
	matches := trackingRE.FindAllStringSubmatch(combined, 1)
	if len(matches) == 0 {
		return ""
	}

	return matches[0][1]
}

// ChangeChannel updates the tracked channel for an installed snap.
func (snap *Snap) ChangeChannel(pack, channel string) error {
	cmd := fmt.Sprintf("snap refresh --channel %s %s", channel, pack)
	out, _, err := RunCommandWithRetry(cmd, snap, snap.retryPolicy)
	if err != nil {
		return err
	} else if strings.Contains(combinedOutput(out, err), "not installed") {
		return errors.Errorf("snap not installed")
	}

	return nil
}

// Install is defined on the PackageManager interface.
func (snap *Snap) Install(packs ...string) error {
	out, _, err := RunCommandWithRetry(snap.cmder.InstallCmd(packs...), snap.installRetryable, snap.retryPolicy)
	if snapNotFoundRE.MatchString(combinedOutput(out, err)) {
		return errors.New("unable to locate package")
	}
	return err
}

func combinedOutput(out string, err error) string {
	res := out
	if err != nil {
		res += err.Error()
	}
	return res
}
