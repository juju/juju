// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasupgraderembedded

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v2/arch"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v2/catacomb"

	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/watcher"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// Upgrader represents a worker that watches the state for upgrade
// requests for a given CAAS agent.
type Upgrader struct {
	catacomb catacomb.Catacomb

	upgraderClient   UpgraderClient
	embeddedUpgrader CAASEmbeddedUpgrader
	tag              names.Tag
	config           Config
}

// UpgraderClient provides the facade methods used by the worker.
type UpgraderClient interface {
	DesiredVersion(tag string) (version.Number, error)
	SetVersion(tag string, v version.Binary) error
	WatchAPIVersion(agentTag string) (watcher.NotifyWatcher, error)
}

// CAASEmbeddedUpgrader provides method to upgrade the embedded workloads.
type CAASEmbeddedUpgrader interface {
	Upgrade(agentTag string, v version.Number) error
}

// Config contains the items the worker needs to start.
type Config struct {
	UpgraderClient              UpgraderClient
	CAASEmbeddedUpgrader        CAASEmbeddedUpgrader
	AgentTag                    names.Tag
	OrigAgentVersion            version.Number
	UpgradeStepsWaiter          gate.Waiter
	InitialUpgradeCheckComplete gate.Unlocker

	Logger Logger
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.UpgraderClient == nil {
		return errors.NotValidf("missing UpgraderClient")
	}
	if config.CAASEmbeddedUpgrader == nil {
		return errors.NotValidf("missing CAASEmbeddedUpgrader")
	}
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

// NewUpgrader returns a new upgrader worker. It watches changes to the
// current version of a CAAS agent. If an upgrade is needed, the worker
// updates the docker image version for the specified agent.
// TODO(caas) - support HA controllers
func NewUpgrader(config Config) (*Upgrader, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	u := &Upgrader{
		upgraderClient:   config.UpgraderClient,
		embeddedUpgrader: config.CAASEmbeddedUpgrader,
		tag:              config.AgentTag,
		config:           config,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &u.catacomb,
		Work: u.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
}

// Kill implements worker.Worker.Kill.
func (u *Upgrader) Kill() {
	u.catacomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (u *Upgrader) Wait() error {
	return u.catacomb.Wait()
}

func (u *Upgrader) loop() error {
	// Only controllers set their version here - agents do it in the main agent worker loop.
	hostOSType := coreos.HostOSTypeName()
	if err := u.upgraderClient.SetVersion(u.tag.String(), toBinaryVersion(jujuversion.Current, hostOSType)); err != nil {
		return errors.Annotate(err, "cannot set agent version")
	}

	// TODO(embedded): implement containeragent upgrade.
	u.config.Logger.Warningf("containeragent upgrade not implemented yet!!!")
	select {}
}

func toBinaryVersion(vers version.Number, osType string) version.Binary {
	outVers := version.Binary{
		Number:  vers,
		Arch:    arch.HostArch(),
		Release: osType,
	}
	return outVers
}
