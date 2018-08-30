// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/os/series"

	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/systemd"
)

//go:generate mockgen -package mocks -destination mocks/servicemanager_mock.go github.com/juju/juju/service SystemdServiceManager

var (
	systemdDir          = systemd.EtcSystemdDir
	systemdMultiUserDir = systemd.EtcSystemdMultiUserDir
)

var hostSeries = series.HostSeries

type seriesGetter = func() (string, error)

type Upgrader interface {
	PerformUpgrade() error
}

// Upgrader handles host machine concerns required to
// upgrade the version of Ubuntu.
type upgrader struct {
	logger Logger

	fromSeries string
	fromInit   string
	toSeries   string
	toInit     string

	machineAgent string
	unitAgents   []string

	manager service.SystemdServiceManager
}

// NewUpgrader uses the input function to determine the series that should be
// supported, and returns a reference to a new Upgrader that supports it.
func NewUpgrader(
	targetSeries seriesGetter,
	manager service.SystemdServiceManager,
	logger Logger,
) (Upgrader, error) {
	fromSeries, err := hostSeries()
	if err != nil {
		return nil, errors.Trace(err)
	}
	fromInit, err := service.VersionInitSystem(fromSeries)
	if err != nil {
		return nil, errors.Trace(err)
	}

	toSeries, err := targetSeries()
	if err != nil {
		return nil, errors.Trace(err)
	}
	toInit, err := service.VersionInitSystem(toSeries)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &upgrader{
		logger:     logger,
		fromSeries: fromSeries,
		fromInit:   fromInit,
		toSeries:   toSeries,
		toInit:     toInit,
		manager:    manager,
	}, nil
}

// PerformUpgrade writes Juju binaries and service files that allow the machine
// and unit agents to run on the target version of Ubuntu.
func (u *upgrader) PerformUpgrade() error {
	if err := u.populateAgents(); err != nil {
		return errors.Trace(err)
	}

	if err := u.ensureSystemdFiles(); err != nil {
		return errors.Trace(err)
	}

	// TODO (manadart 2018-08-29): Write agent binaries.

	return nil
}

// Populate agents discovers and sets the names of the machine and unit agents.
// If there are any other agents determined, a warning is logged to notify that
// they are being skipped from the upgrade process.
func (u *upgrader) populateAgents() (err error) {
	var unknown []string
	u.machineAgent, u.unitAgents, unknown, err = u.manager.FindAgents(paths.NixDataDir)
	if err != nil {
		return errors.Trace(err)
	}
	if len(unknown) > 0 {
		u.logger.Warningf("skipping agents not of type machine or unit: %s", strings.Join(unknown, ", "))
	}
	return nil
}

// ensureSystemdFiles determines whether re-writing service files to target
// systemd is required. If it is, the necessary changes are invoked via the
// service manager.
func (u *upgrader) ensureSystemdFiles() (err error) {
	if u.fromInit == service.InitSystemSystemd || u.toInit != service.InitSystemSystemd {
		return nil
	}

	services, links, failed, err := u.manager.WriteSystemdAgents(
		u.machineAgent, u.unitAgents, paths.NixDataDir, systemdDir, systemdMultiUserDir)

	if len(services) > 0 {
		u.logger.Infof("agents written and linked by systemd: %s", strings.Join(services, ", "))
	}
	if len(links) > 0 {
		u.logger.Infof("agents written and linked by symlink: %s", strings.Join(links, ", "))
	}

	return errors.Annotatef(err, "failed to write agents: %s", strings.Join(failed, ", "))
}
