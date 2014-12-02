// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	corecharm "gopkg.in/juju/charm.v4"

	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
)

// NewFactory returns a Factory that creates Operations backed by the supplied
// parameters.
func NewFactory(
	deployer charm.Deployer,
	contextFactory context.Factory,
	callbacks Callbacks,
	abort <-chan struct{},
) Factory {
	return &factory{
		deployer:       deployer,
		contextFactory: contextFactory,
		callbacks:      callbacks,
		abort:          abort,
	}
}

type factory struct {
	deployer       charm.Deployer
	contextFactory context.Factory
	callbacks      Callbacks
	abort          <-chan struct{}
}

// NewDeploy is part of the Factory interface.
func (f *factory) NewDeploy(charmURL *corecharm.URL, kind Kind) (Operation, error) {
	if charmURL == nil {
		return nil, errors.New("charm url required")
	} else if kind != Install && kind != Upgrade {
		return nil, errors.Errorf("unknown deploy kind: %s", kind)
	}
	return &deploy{
		kind:      kind,
		charmURL:  charmURL,
		callbacks: f.callbacks,
		deployer:  f.deployer,
		abort:     f.abort,
	}, nil
}

// NewHook is part of the Factory interface.
func (f *factory) NewHook(hookInfo hook.Info) (Operation, error) {
	if err := hookInfo.Validate(); err != nil {
		return nil, err
	}
	return &runHook{
		info:           hookInfo,
		callbacks:      f.callbacks,
		contextFactory: f.contextFactory,
	}, nil
}

// NewAction is part of the Factory interface.
func (f *factory) NewAction(actionId string) (Operation, error) {
	if !names.IsValidAction(actionId) {
		return nil, errors.Errorf("invalid action id %q", actionId)
	}
	return &runAction{
		actionId:       actionId,
		callbacks:      f.callbacks,
		contextFactory: f.contextFactory,
	}, nil
}

// NewCommands is part of the Factory interface.
func (f *factory) NewCommands(commands string, relationId int, remoteUnitName string, sendResponse CommandResponseFunc) (Operation, error) {
	if commands == "" {
		return nil, errors.New("commands required")
	} else if sendResponse == nil {
		return nil, errors.New("response sender required")
	}
	if remoteUnitName != "" {
		if relationId == -1 {
			return nil, errors.New("remote unit not valid without relation")
		} else if !names.IsValidUnit(remoteUnitName) {
			return nil, errors.Errorf("invalid remote unit name %q", remoteUnitName)
		}
	}
	return &runCommands{
		commands:       commands,
		relationId:     relationId,
		remoteUnitName: remoteUnitName,
		sendResponse:   sendResponse,
		callbacks:      f.callbacks,
		contextFactory: f.contextFactory,
	}, nil
}
