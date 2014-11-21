// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	corecharm "gopkg.in/juju/charm.v4"

	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/context"
	"github.com/juju/juju/worker/uniter/hook"
)

func NewFactory(
	state *apiuniter.State,
	hookHelper RunHookHelper,
	paths context.Paths,
	contextFactory context.Factory,
	acquireLock func(string) (func(), error),
	setCharm func(*corecharm.URL) error,
	deployer charm.Deployer,
	abort <-chan struct{},
) Factory {
	return &factory{
		state:          state,
		hookHelper:     hookHelper,
		paths:          paths,
		contextFactory: contextFactory,
		acquireLock:    acquireLock,
		setCharm:       setCharm,
		deployer:       deployer,
		abort:          abort,
	}
}

type factory struct {
	state          *apiuniter.State
	hookHelper     RunHookHelper
	paths          context.Paths
	contextFactory context.Factory
	acquireLock    func(string) (func(), error)
	setCharm       func(*corecharm.URL) error
	deployer       charm.Deployer
	abort          <-chan struct{}
}

func (f *factory) NewDeploy(charmURL *corecharm.URL, kind Kind) (Operation, error) {
	if charmURL == nil {
		return nil, errors.New("charm url required")
	} else if kind != Install && kind != Upgrade {
		return nil, errors.Errorf("unknown deploy kind: %s", kind)
	}
	return &deploy{
		kind:     kind,
		charmURL: charmURL,
		getInfo:  f.getCharmInfo,
		setCharm: f.setCharm,
		deployer: f.deployer,
		abort:    f.abort,
	}, nil
}

func (f *factory) getCharmInfo(charmURL *corecharm.URL) (charm.BundleInfo, error) {
	ch, err := f.state.Charm(charmURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ch, nil
}

func (f *factory) NewHook(hookInfo hook.Info) (Operation, error) {
	if err := hookInfo.Validate(); err != nil {
		return nil, err
	}
	return &runHook{
		info:           hookInfo,
		helper:         f.hookHelper,
		contextFactory: f.contextFactory,
		paths:          f.paths,
		acquireLock:    f.acquireLock,
	}, nil
}

func (f *factory) NewAction(actionId string) (Operation, error) {
	actionTag, ok := names.ParseActionTagFromId(actionId)
	if !ok {
		return nil, errors.Errorf("invalid action id %q", actionId)
	}
	return &runAction{
		state:          f.state,
		actionId:       actionId,
		actionTag:      actionTag,
		contextFactory: f.contextFactory,
		paths:          f.paths,
		acquireLock:    f.acquireLock,
	}, nil
}

func (f *factory) NewCommands(commands string, sendResponse CommandResponseFunc) (Operation, error) {
	if sendResponse == nil {
		return nil, errors.New("response sender required")
	}
	return &runCommands{
		commands:       commands,
		sendResponse:   sendResponse,
		contextFactory: f.contextFactory,
		paths:          f.paths,
		acquireLock:    f.acquireLock,
	}, nil
}
