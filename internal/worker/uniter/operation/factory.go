// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/runner"
	"github.com/juju/juju/rpc/params"
)

// FactoryParams holds all the necessary parameters for a new operation factory.
type FactoryParams struct {
	Deployer       charm.Deployer
	RunnerFactory  runner.Factory
	Callbacks      Callbacks
	ActionGetter   ActionGetter
	MetricSpoolDir string
	Logger         logger.Logger
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
func (f *factory) newDeploy(kind Kind, charmURL string, revert, resolved bool) (Operation, error) {
	if f.config.Deployer == nil {
		return nil, errors.New("deployer required")
	}
	if charmURL == "" {
		return nil, errors.New("charm url required")
	} else if kind != Install && kind != Upgrade {
		return nil, errors.Errorf("unknown deploy kind: %s", kind)
	}
	var op Operation = &deploy{
		kind:      kind,
		charmURL:  charmURL,
		revert:    revert,
		resolved:  resolved,
		callbacks: f.config.Callbacks,
		deployer:  f.config.Deployer,
	}
	return op, nil
}

// NewInstall is part of the Factory interface.
func (f *factory) NewInstall(charmURL string) (Operation, error) {
	return f.newDeploy(Install, charmURL, false, false)
}

// NewUpgrade is part of the Factory interface.
func (f *factory) NewUpgrade(charmURL string) (Operation, error) {
	return f.newDeploy(Upgrade, charmURL, false, false)
}

// NewRevertUpgrade is part of the Factory interface.
func (f *factory) NewRevertUpgrade(charmURL string) (Operation, error) {
	return f.newDeploy(Upgrade, charmURL, true, false)
}

// NewResolvedUpgrade is part of the Factory interface.
func (f *factory) NewResolvedUpgrade(charmURL string) (Operation, error) {
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
		logger:        f.config.Logger,
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

// NewNoOpSecretsRemoved is part of the Factory interface.
func (f *factory) NewNoOpSecretsRemoved(uris []string) (Operation, error) {
	return &noOpSecretsRemoved{
		Operation: &skipOperation{}, uris: uris,
		callbacks: f.config.Callbacks,
	}, nil
}

// NewAction is part of the Factory interface.
func (f *factory) NewAction(ctx context.Context, actionId string) (Operation, error) {
	if !names.IsValidAction(actionId) {
		return nil, errors.Errorf("invalid action id %q", actionId)
	}

	tag := names.NewActionTag(actionId)
	action, err := f.config.ActionGetter.Action(ctx, tag)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return nil, charmrunner.ErrActionNotAvailable
	} else if params.IsCodeActionNotAvailable(err) {
		return nil, charmrunner.ErrActionNotAvailable
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	return &runAction{
		action:        action,
		callbacks:     f.config.Callbacks,
		runnerFactory: f.config.RunnerFactory,
		logger:        f.config.Logger,
	}, nil
}

// NewFailAction is part of the factory interface.
func (f *factory) NewFailAction(actionId string) (Operation, error) {
	if !names.IsValidAction(actionId) {
		return nil, errors.Errorf("invalid action id %q", actionId)
	}
	return &failAction{
		actionId:  actionId,
		callbacks: f.config.Callbacks,
	}, nil
}

// NewCommands is part of the Factory interface.
func (f *factory) NewCommands(args CommandArgs, sendResponse CommandResponseFunc) (Operation, error) {
	if err := args.Validate(); err != nil {
		return nil, errors.Trace(err)
	} else if sendResponse == nil {
		return nil, errors.New("response sender required")
	}
	return &runCommands{
		args:          args,
		sendResponse:  sendResponse,
		callbacks:     f.config.Callbacks,
		runnerFactory: f.config.RunnerFactory,
		logger:        f.config.Logger,
	}, nil
}

// NewResignLeadership is part of the Factory interface.
func (f *factory) NewResignLeadership() (Operation, error) {
	return &resignLeadership{logger: f.config.Logger}, nil
}

// NewAcceptLeadership is part of the Factory interface.
func (f *factory) NewAcceptLeadership() (Operation, error) {
	return &acceptLeadership{}, nil
}
