// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/proxy"

	"github.com/juju/juju/internal/errors"
)

const (
	// snap binary name.
	snapBinary = "snap"

	snapProxySettingFormat = `proxy.%s=%q`
)

// NewSnapPackageCommander returns a PackageCommander for snap-based systems.
func NewSnapPackageCommander() SnapPackageCommander {
	return SnapPackageCommander{}
}

// SnapPackageCommander provides runnable shell commands for snap-based package
// operations.
type SnapPackageCommander struct{}

// UpdateCmd returns the command to update the local package list.
func (SnapPackageCommander) UpgradeCmd() string {
	return buildCommand(snapBinary, "refresh")
}

// InstallCmd returns a *single* command that installs the given package(s).
func (SnapPackageCommander) InstallCmd(packs ...string) string {
	args := append([]string{snapBinary, "install"}, packs...)
	return buildCommand(args...)
}

var (
	// ErrProxySettingNotSupported is returned when a proxy setting is not supported by snap.
	ErrProxySettingNotSupported = errors.New("proxy setting not supported by snap")
)

// SetProxyCmds returns the commands which configure snap's proxy configuration
func (SnapPackageCommander) SetProxyCmds(settings proxy.Settings) ([]string, error) {
	if settings.Ftp != "" {
		return nil, errors.Errorf("ftp: %w", ErrProxySettingNotSupported)
	}
	if settings.NoProxy != "" {
		return nil, errors.Errorf("NoProxy: %w", ErrProxySettingNotSupported)
	}
	if settings.AutoNoProxy != "" {
		return nil, errors.Errorf("AutoNoProxy: %w", ErrProxySettingNotSupported)
	}

	commands := []string{}
	if settings.Http != "" {
		proxy := fmt.Sprintf(snapProxySettingFormat, "http", settings.Http)
		commands = append(commands, buildCommand(snapBinary, "set system", proxy))
	}
	if settings.Https != "" {
		proxy := fmt.Sprintf(snapProxySettingFormat, "https", settings.Https)
		commands = append(commands, buildCommand(snapBinary, "set system", proxy))
	}
	return commands, nil
}
