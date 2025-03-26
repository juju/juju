// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasupgrader

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4/catacomb"

	coreagent "github.com/juju/juju/core/agent"
	"github.com/juju/juju/core/arch"
	coreos "github.com/juju/juju/core/os"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/upgrader"
)

var logger = internallogger.GetLogger("juju.worker.caasupgrader")

// Upgrader represents a worker that watches the state for upgrade
// requests for a given CAAS agent.
type Upgrader struct {
	catacomb catacomb.Catacomb

	upgraderClient   UpgraderClient
	operatorUpgrader CAASOperatorUpgrader
	tag              names.Tag
	config           Config
}

// UpgraderClient provides the facade methods used by the worker.
type UpgraderClient interface {
	DesiredVersion(ctx context.Context, tag string) (version.Number, error)
	SetVersion(ctx context.Context, tag string, v version.Binary) error
	WatchAPIVersion(ctx context.Context, agentTag string) (watcher.NotifyWatcher, error)
}

type CAASOperatorUpgrader interface {
	Upgrade(ctx context.Context, agentTag string, v version.Number) error
}

// Config contains the items the worker needs to start.
type Config struct {
	UpgraderClient              UpgraderClient
	CAASOperatorUpgrader        CAASOperatorUpgrader
	AgentTag                    names.Tag
	OrigAgentVersion            version.Number
	UpgradeStepsWaiter          gate.Waiter
	InitialUpgradeCheckComplete gate.Unlocker
}

// NewUpgrader returns a new upgrader worker. It watches changes to the
// current version of a CAAS agent. If an upgrade is needed, the worker
// updates the docker image version for the specified agent.
// TODO(caas) - support HA controllers
func NewUpgrader(config Config) (*Upgrader, error) {
	u := &Upgrader{
		upgraderClient:   config.UpgraderClient,
		operatorUpgrader: config.CAASOperatorUpgrader,
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
	ctx, cancel := u.scopedContext()
	defer cancel()

	// Only controllers and sidecar unit agents set their version here - application agents do it in the main agent worker loop.
	hostOSType := coreos.HostOSTypeName()
	if coreagent.IsAllowedControllerTag(u.tag.Kind()) || u.tag.Kind() == names.UnitTagKind {
		if err := u.upgraderClient.SetVersion(ctx, u.tag.String(), toBinaryVersion(jujuversion.Current, hostOSType)); err != nil {
			return errors.Annotatef(err, "setting agent version for %q", u.tag.String())
		}
	}

	// We don't read on the dying channel until we have received the
	// initial event from the API version watcher, thus ensuring
	// that we attempt an upgrade even if other workers are dying
	// all around us. Similarly, we don't want to bind the watcher
	// to the catacomb's lifetime (yet!) lest we wait forever for a
	// stopped watcher.
	//
	// However, that absolutely depends on versionWatcher's guaranteed
	// initial event, and we should assume that it'll break its contract
	// sometime. So we allow the watcher to wait patiently for the event
	// for a full minute; but after that we proceed regardless.
	versionWatcher, err := u.upgraderClient.WatchAPIVersion(ctx, u.tag.String())
	if err != nil {
		return errors.Annotate(err, "getting upgrader facade watch api version client")
	}
	logger.Infof(ctx, "abort check blocked until version event received")
	// TODO(fwereade): 2016-03-17 lp:1558657
	mustProceed := time.After(time.Minute)
	var dying <-chan struct{}
	allowDying := func() {
		if dying == nil {
			logger.Infof(ctx, "unblocking abort check")
			mustProceed = nil
			dying = u.catacomb.Dying()
			if err := u.catacomb.Add(versionWatcher); err != nil {
				u.catacomb.Kill(err)
			}
		}
	}

	logger.Debugf(ctx, "current agent binary version: %v", jujuversion.Current)
	for {
		select {
		// NOTE: dying starts out nil, so it can't be chosen
		// first time round the loop. However...
		case <-dying:
			return u.catacomb.ErrDying()
		// ...*every* other case *must* allowDying(), before doing anything
		// else, lest an error cause us to leak versionWatcher.
		case <-mustProceed:
			logger.Infof(ctx, "version event not received after one minute")
			allowDying()
		case _, ok := <-versionWatcher.Changes():
			allowDying()
			if !ok {
				return errors.New("version watcher closed")
			}
		}

		wantVersion, err := u.upgraderClient.DesiredVersion(ctx, u.tag.String())
		if err != nil {
			return err
		}

		if wantVersion == jujuversion.Current {
			u.config.InitialUpgradeCheckComplete.Unlock()
			continue
		} else if !upgrader.AllowedTargetVersion(jujuversion.Current, wantVersion) {
			logger.Warningf(ctx, "desired agent binary version: %s is older than current %s, refusing to downgrade",
				wantVersion, jujuversion.Current)
			u.config.InitialUpgradeCheckComplete.Unlock()
			continue
		}
		direction := "upgrade"
		if wantVersion.Compare(jujuversion.Current) == -1 {
			direction = "downgrade"
		}
		logger.Debugf(ctx, "%s requested for %v from %v to %v", direction, u.tag, jujuversion.Current, wantVersion)
		err = u.operatorUpgrader.Upgrade(ctx, u.tag.String(), wantVersion)
		if err != nil {
			return errors.Annotatef(
				err, "requesting upgrade for %v from %v to %v", u.tag.String(), jujuversion.Current, wantVersion)
		}
	}
}

func (u *Upgrader) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(u.catacomb.Context(context.Background()))
}

func toBinaryVersion(vers version.Number, osType string) version.Binary {
	outVers := version.Binary{
		Number:  vers,
		Arch:    arch.HostArch(),
		Release: osType,
	}
	return outVers
}
