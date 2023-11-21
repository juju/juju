// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/common/charmrunner"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/runner"
)

// FactoryParams holds all the necessary parameters for a new operation factory.
type FactoryParams struct {
	Deployer       charm.Deployer
	RunnerFactory  runner.Factory
	Callbacks      Callbacks
	ActionGetter   ActionGetter
	Abort          <-chan struct{}
	MetricSpoolDir string
	Logger         Logger
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
	return &tracedOperation{Operation: &deploy{
		kind:      kind,
		charmURL:  charmURL,
		revert:    revert,
		resolved:  resolved,
		callbacks: f.config.Callbacks,
		deployer:  f.config.Deployer,
		abort:     f.config.Abort,
	}}, nil
}

// NewInstall is part of the Factory interface.
func (f *factory) NewInstall(charmURL string) (Operation, error) {
	return f.newDeploy(Install, charmURL, false, false)
}

// NewUpgrade is part of the Factory interface.
func (f *factory) NewUpgrade(charmURL string) (Operation, error) {
	return f.newDeploy(Upgrade, charmURL, false, false)
}

// NewRemoteInit is part of the Factory interface.
func (f *factory) NewRemoteInit(runningStatus remotestate.ContainerRunningStatus) (Operation, error) {
	return &tracedOperation{Operation: &remoteInit{
		callbacks:     f.config.Callbacks,
		abort:         f.config.Abort,
		runningStatus: runningStatus,
	}}, nil
}

func (f *factory) NewSkipRemoteInit(retry bool) (Operation, error) {
	return &tracedOperation{Operation: &skipRemoteInit{retry: retry}}, nil
}

func (f *factory) NewNoOpFinishUpgradeSeries() (Operation, error) {
	return &tracedOperation{Operation: &noOpFinishUpgradeSeries{Operation: &skipOperation{}}}, nil
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
	return &tracedOperation{Operation: &runHook{
		info:          hookInfo,
		callbacks:     f.config.Callbacks,
		runnerFactory: f.config.RunnerFactory,
		logger:        f.config.Logger,
	}}, nil
}

// NewSkipHook is part of the Factory interface.
func (f *factory) NewSkipHook(hookInfo hook.Info) (Operation, error) {
	hookOp, err := f.NewRunHook(hookInfo)
	if err != nil {
		return nil, err
	}
	return &tracedOperation{Operation: &skipOperation{
		Operation: hookOp,
	}}, nil
}

// NewNoOpSecretsRemoved is part of the Factory interface.
func (f *factory) NewNoOpSecretsRemoved(uris []string) (Operation, error) {
	return &tracedOperation{Operation: &noOpSecretsRemoved{
		Operation: &skipOperation{}, uris: uris,
		callbacks: f.config.Callbacks,
	}}, nil
}

// NewAction is part of the Factory interface.
func (f *factory) NewAction(actionId string) (Operation, error) {
	if !names.IsValidAction(actionId) {
		return nil, errors.Errorf("invalid action id %q", actionId)
	}

	tag := names.NewActionTag(actionId)
	action, err := f.config.ActionGetter.Action(tag)
	if params.IsCodeNotFoundOrCodeUnauthorized(err) {
		return nil, charmrunner.ErrActionNotAvailable
	} else if params.IsCodeActionNotAvailable(err) {
		return nil, charmrunner.ErrActionNotAvailable
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	return &tracedOperation{Operation: &runAction{
		action:        action,
		callbacks:     f.config.Callbacks,
		runnerFactory: f.config.RunnerFactory,
		logger:        f.config.Logger,
	}}, nil
}

// NewFailAction is part of the factory interface.
func (f *factory) NewFailAction(actionId string) (Operation, error) {
	if !names.IsValidAction(actionId) {
		return nil, errors.Errorf("invalid action id %q", actionId)
	}
	return &tracedOperation{Operation: &failAction{
		actionId:  actionId,
		callbacks: f.config.Callbacks,
	}}, nil
}

// NewCommands is part of the Factory interface.
func (f *factory) NewCommands(args CommandArgs, sendResponse CommandResponseFunc) (Operation, error) {
	if err := args.Validate(); err != nil {
		return nil, errors.Trace(err)
	} else if sendResponse == nil {
		return nil, errors.New("response sender required")
	}
	return &tracedOperation{Operation: &runCommands{
		args:          args,
		sendResponse:  sendResponse,
		callbacks:     f.config.Callbacks,
		runnerFactory: f.config.RunnerFactory,
		logger:        f.config.Logger,
	}}, nil
}

// NewResignLeadership is part of the Factory interface.
func (f *factory) NewResignLeadership() (Operation, error) {
	return &tracedOperation{Operation: &resignLeadership{logger: f.config.Logger}}, nil
}

// NewAcceptLeadership is part of the Factory interface.
func (f *factory) NewAcceptLeadership() (Operation, error) {
	return &tracedOperation{Operation: &acceptLeadership{}}, nil
}

type tracedOperation struct {
	Operation
}

// Prepare ensures that the operation is valid and ready to be executed.
// If it returns a non-nil state, that state will be validated and recorded.
// If it returns ErrSkipExecute, it indicates that the operation can be
// committed directly.
func (o *tracedOperation) Prepare(ctx stdcontext.Context, state State) (_ *State, err error) {
	ctx, span := coretrace.Start(ctx, coretrace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return o.Operation.Prepare(ctx, state)
}

// Execute carries out the operation. It must not be called without having
// called Prepare first. If it returns a non-nil state, that state will be
// validated and recorded.
func (o *tracedOperation) Execute(ctx stdcontext.Context, state State) (_ *State, err error) {
	ctx, span := coretrace.Start(ctx, coretrace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return o.Operation.Execute(ctx, state)
}

// Commit ensures that the operation's completion is recorded. If it returns
// a non-nil state, that state will be validated and recorded.
func (o *tracedOperation) Commit(ctx stdcontext.Context, state State) (_ *State, err error) {
	ctx, span := coretrace.Start(ctx, coretrace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return o.Operation.Commit(ctx, state)
}
