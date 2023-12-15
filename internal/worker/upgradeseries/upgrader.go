// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries

import (
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/service"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/servicemanager_mock.go github.com/juju/juju/internal/service SystemdServiceManager

// Upgrader describes methods required to perform file-system manipulation in
// preparation for upgrading the host Ubuntu version.
type Upgrader interface {
	PerformUpgrade() error
}

// upgrader implements the Upgrader interface for a specific (from/to) upgrade
// of the host Ubuntu version, via the systemd service manager.
type upgrader struct {
	logger Logger

	// jujuCurrentSeries is what Juju thinks the
	// current series of the machine is.
	jujuCurrentSeries string

	// fromSeries is the actual current series,
	// determined directly from the machine.
	fromSeries string

	toSeries string

	machineAgent string
	unitAgents   []string

	manager service.SystemdServiceManager
}

// NewUpgrader uses the input function to determine the series that should be
// supported, and returns a reference to a new Upgrader that supports it.
func NewUpgrader(
	currentSeries, toSeries string, manager service.SystemdServiceManager, logger Logger,
) (Upgrader, error) {
	fromSeries, err := hostSeries()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &upgrader{
		logger:            logger,
		jujuCurrentSeries: currentSeries,
		fromSeries:        fromSeries,
		toSeries:          toSeries,
		manager:           manager,
	}, nil
}

// PerformUpgrade writes Juju binaries and service files that allow the machine
// and unit agents to run on the target version of Ubuntu.
func (u *upgrader) PerformUpgrade() error {
	return u.populateAgents()
}

// populateAgents discovers and sets the names of the machine and unit agents.
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
