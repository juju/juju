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

func NewFactory(
	paths context.Paths,
	deployer charm.Deployer,
	contextFactory context.Factory,
	callbacks Callbacks,
	abort <-chan struct{},
) Factory {
	return &factory{
		paths:          paths,
		deployer:       deployer,
		contextFactory: contextFactory,
		callbacks:      callbacks,
		abort:          abort,
	}
}

type factory struct {
	paths          context.Paths
	deployer       charm.Deployer
	contextFactory context.Factory
	callbacks      Callbacks
	abort          <-chan struct{}
}

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

func (f *factory) NewHook(hookInfo hook.Info) (Operation, error) {
	if err := hookInfo.Validate(); err != nil {
		return nil, err
	}
	return &runHook{
		info:           hookInfo,
		paths:          f.paths,
		callbacks:      f.callbacks,
		contextFactory: f.contextFactory,
	}, nil
}

func (f *factory) NewAction(actionId string) (Operation, error) {
	if !names.IsValidAction(actionId) {
		return nil, errors.Errorf("invalid action id %q", actionId)
	}
	return &runAction{
		actionId:       actionId,
		paths:          f.paths,
		callbacks:      f.callbacks,
		contextFactory: f.contextFactory,
	}, nil
}

func (f *factory) NewCommands(commands string, sendResponse CommandResponseFunc) (Operation, error) {
	if commands == "" {
		return nil, errors.New("commands required")
	} else if sendResponse == nil {
		return nil, errors.New("response sender required")
	}
	return &runCommands{
		commands:       commands,
		sendResponse:   sendResponse,
		paths:          f.paths,
		callbacks:      f.callbacks,
		contextFactory: f.contextFactory,
	}, nil
}
