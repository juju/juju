// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	corecharm "gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/runner"
)

// FactoryParams holds all the necessary parameters for a new operation factory.
type FactoryParams struct {
	Deployer       charm.Deployer
	RunnerFactory  runner.Factory
	Callbacks      Callbacks
	StorageUpdater StorageUpdater
	Abort          <-chan struct{}
	MetricSpoolDir string
}

// NewFactory returns a Factory that creates Operations backed by the supplied
// parameters.
func NewFactory(params FactoryParams) Factory {
	return &factory{
		config: params,
	}
}

type factory struct {
	config FactoryParams
}

// newDeploy is the common code for creating arbitrary deploy operations.
func (f *factory) newDeploy(kind Kind, charmURL *corecharm.URL, revert, resolved bool) (Operation, error) {
	if charmURL == nil {
		return nil, errors.New("charm url required")
	} else if kind != Install && kind != Upgrade {
		return nil, errors.Errorf("unknown deploy kind: %s", kind)
	}
	return &deploy{
		kind:      kind,
		charmURL:  charmURL,
		revert:    revert,
		resolved:  resolved,
		callbacks: f.config.Callbacks,
		deployer:  f.config.Deployer,
		abort:     f.config.Abort,
	}, nil
}

// NewInstall is part of the Factory interface.
func (f *factory) NewInstall(charmURL *corecharm.URL) (Operation, error) {
	return f.newDeploy(Install, charmURL, false, false)
}

// NewUpgrade is part of the Factory interface.
func (f *factory) NewUpgrade(charmURL *corecharm.URL) (Operation, error) {
	return f.newDeploy(Upgrade, charmURL, false, false)
}

// NewRevertUpgrade is part of the Factory interface.
func (f *factory) NewRevertUpgrade(charmURL *corecharm.URL) (Operation, error) {
	return f.newDeploy(Upgrade, charmURL, true, false)
}

// NewResolvedUpgrade is part of the Factory interface.
func (f *factory) NewResolvedUpgrade(charmURL *corecharm.URL) (Operation, error) {
	return f.newDeploy(Upgrade, charmURL, false, true)
}

// NewRunHook is part of the Factory interface.
func (f *factory) NewRunHook(hookInfo hook.Info) (Operation, error) {
	if err := hookInfo.Validate(); err != nil {
		return nil, err
	}
	return &runHook{
		info:          hookInfo,
		callbacks:     f.config.Callbacks,
		runnerFactory: f.config.RunnerFactory,
	}, nil
}

// NewSkipHook is part of the Factory interface.
func (f *factory) NewSkipHook(hookInfo hook.Info) (Operation, error) {
	hookOp, err := f.NewRunHook(hookInfo)
	if err != nil {
		return nil, err
	}
	return &skipOperation{hookOp}, nil
}

// NewAction is part of the Factory interface.
func (f *factory) NewAction(actionId string) (Operation, error) {
	if !names.IsValidAction(actionId) {
		return nil, errors.Errorf("invalid action id %q", actionId)
	}
	return &runAction{
		actionId:      actionId,
		callbacks:     f.config.Callbacks,
		runnerFactory: f.config.RunnerFactory,
	}, nil
}

// NewCommands is part of the Factory interface.
func (f *factory) NewCommands(args CommandArgs, sendResponse CommandResponseFunc) (Operation, error) {
	if args.Commands == "" {
		return nil, errors.New("commands required")
	} else if sendResponse == nil {
		return nil, errors.New("response sender required")
	}
	if args.RemoteUnitName != "" {
		if args.RelationId == -1 {
			return nil, errors.New("remote unit not valid without relation")
		} else if !names.IsValidUnit(args.RemoteUnitName) {
			return nil, errors.Errorf("invalid remote unit name %q", args.RemoteUnitName)
		}
	}
	return &runCommands{
		args:          args,
		sendResponse:  sendResponse,
		callbacks:     f.config.Callbacks,
		runnerFactory: f.config.RunnerFactory,
	}, nil
}

// NewUpdateStorage is part of the Factory interface.
func (f *factory) NewUpdateStorage(tags []names.StorageTag) (Operation, error) {
	return &updateStorage{
		tags:           tags,
		storageUpdater: f.config.StorageUpdater,
	}, nil
}

// NewResignLeadership is part of the Factory interface.
func (f *factory) NewResignLeadership() (Operation, error) {
	return &resignLeadership{}, nil
}

// NewAcceptLeadership is part of the Factory interface.
func (f *factory) NewAcceptLeadership() (Operation, error) {
	return &acceptLeadership{}, nil
}
