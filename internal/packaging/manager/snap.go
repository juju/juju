// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package manager

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/internal/packaging/commands"
	"github.com/juju/proxy"
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

	// snapProxyRe is a regexp which matches all proxy-related configuration
	// options in the snap proxy settings output
	snapProxyRE = regexp.MustCompile(`(?im)^proxy\.(?P<protocol>[a-z]+)\s+(?P<proxy>.+)$`)

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

// Search is defined on the PackageManager interface.
func (snap *Snap) Search(pack string) (bool, error) {
	out, _, err := RunCommandWithRetry(snap.cmder.SearchCmd(pack), snap, snap.retryPolicy)
	if strings.Contains(combinedOutput(out, err), "error: no snap found") {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return true, nil
}

// IsInstalled is defined on the PackageManager interface.
func (snap *Snap) IsInstalled(pack string) bool {
	out, _, err := RunCommandWithRetry(snap.cmder.IsInstalledCmd(pack), snap, snap.retryPolicy)
	if strings.Contains(combinedOutput(out, err), "error: no matching snaps installed") || err != nil {
		return false
	}
	return true
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

// GetProxySettings is defined on the PackageManager interface.
func (snap *Snap) GetProxySettings() (proxy.Settings, error) {
	var res proxy.Settings

	out, _, err := RunCommandWithRetry(snap.cmder.GetProxyCmd(), snap, snap.retryPolicy)
	if strings.Contains(combinedOutput(out, err), `no "proxy" configuration option`) {
		return res, nil
	} else if err != nil {
		return res, err
	}

	for _, match := range snapProxyRE.FindAllStringSubmatch(out, -1) {
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

// ConfigureStoreProxy sets up snapd to connect to the snap store proxy
// instance defined in the provided assertions using the provided store ID.
//
// If snap also needs to use HTTP/HTTPS proxies to talk to the outside world,
// these need to be configured separately before invoking this method via a
// call to SetProxy.
func (snap *Snap) ConfigureStoreProxy(assertions, storeID string) error {
	// Setup proxy based on the instructions from:
	// https://docs.ubuntu.com/snap-store-proxy/en/devices
	//
	// Note that while the above instructions run "snap ack /dev/stdin" the
	// code below will instead write the assertions to a temp file and pass
	// that to snap ack. This is purely done to make testing easier.
	assertFile, err := ioutil.TempFile("", "assertions")
	if err != nil {
		return errors.Annotate(err, "unable to create assertion file")
	}
	defer func() {
		_ = assertFile.Close()
		_ = os.Remove(assertFile.Name())
	}()
	if _, err = assertFile.WriteString(assertions); err != nil {
		return errors.Annotate(err, "unable to write to assertion file")
	}
	_ = assertFile.Close()

	ackCmd := fmt.Sprintf("snap ack %s", assertFile.Name())
	if _, _, err = RunCommandWithRetry(ackCmd, snap, snap.retryPolicy); err != nil {
		return errors.Annotate(err, "failed to execute 'snap ack'")
	}

	setCmd := fmt.Sprintf("snap set system proxy.store=%s", storeID)
	if _, _, err = RunCommandWithRetry(setCmd, snap, snap.retryPolicy); err != nil {
		return errors.Annotatef(err, "failed to configure snap to use store ID %q", storeID)
	}

	return nil
}

// DisableStoreProxy resets the snapd proxy store settings.
//
// If snap was also configured to use HTTP/HTTPS proxies these must be reset
// separately via a call to SetProxy.
// call to SetProxy.
func (snap *Snap) DisableStoreProxy() error {
	setCmd := "snap set system proxy.store="
	if _, _, err := RunCommandWithRetry(setCmd, snap, snap.retryPolicy); err != nil {
		return errors.Annotate(err, "failed to configure snap to not use a store proxy")
	}

	return nil
}

func combinedOutput(out string, err error) string {
	res := out
	if err != nil {
		res += err.Error()
	}
	return res
}
