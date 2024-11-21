// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"context"
	stdcontext "context"
	"fmt"
	"runtime/pprof"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
)

type executorStep struct {
	verb string
	run  func(op Operation, ctx context.Context, state State) (*State, error)
}

func (step executorStep) message(op Operation, unitName string) string {
	return fmt.Sprintf("%s operation %q for %s", step.verb, op, unitName)
}

var (
	stepPrepare = executorStep{"preparing", Operation.Prepare}
	stepExecute = executorStep{"executing", Operation.Execute}
	stepCommit  = executorStep{"committing", Operation.Commit}
)

type executor struct {
	unitName           string
	stateOps           *StateOps
	state              *State
	acquireMachineLock func(string, string) (func(), error)
	logger             logger.Logger
}

// ExecutorConfig defines configuration for an Executor.
type ExecutorConfig struct {
	StateReadWriter UnitStateReadWriter
	InitialState    State
	AcquireLock     func(string, string) (func(), error)
	Logger          logger.Logger
}

func (e ExecutorConfig) validate() error {
	if e.StateReadWriter == nil {
		return errors.NotValidf("executor config with nil state ops")
	}
	if e.Logger == nil {
		return errors.NotValidf("executor config with nil logger")
	}
	return nil
}

// NewExecutor returns an Executor which takes its starting state from
// the controller, and records state changes there. If no saved state
// exists, the executor's starting state will be the supplied InitialState.
func NewExecutor(ctx stdcontext.Context, unitName string, cfg ExecutorConfig) (Executor, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	stateOps := NewStateOps(cfg.StateReadWriter)
	state, err := stateOps.Read(ctx)
	if err == ErrNoSavedState {
		state = &cfg.InitialState
	} else if err != nil {
		return nil, err
	}
	return &executor{
		unitName:           unitName,
		stateOps:           stateOps,
		state:              state,
		acquireMachineLock: cfg.AcquireLock,
		logger:             cfg.Logger,
	}, nil
}

// State is part of the Executor interface.
func (x *executor) State() State {
	return *x.state
}

// Run is part of the Executor interface.
func (x *executor) Run(ctx context.Context, op Operation, remoteStateChange <-chan remotestate.Snapshot) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(), trace.WithAttributes(
		trace.StringAttr("executor.state", op.String()),
		trace.StringAttr("executor.unit", x.unitName),
	))
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	pprof.Do(ctx, pprof.Labels(trace.OTELTraceID, span.Scope().TraceID()), func(ctx context.Context) {
		err = x.run(ctx, op, remoteStateChange)
	})
	return
}

// Skip is part of the Executor interface.
func (x *executor) Skip(ctx context.Context, op Operation) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc(), trace.WithAttributes(
		trace.StringAttr("executor.state", op.String()),
		trace.StringAttr("executor.unit", x.unitName),
	))
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	pprof.Do(ctx, pprof.Labels(trace.OTELTraceID, span.Scope().TraceID()), func(ctx context.Context) {
		x.logger.Debugf(context.TODO(), "skipping operation %v for %s", op, x.unitName)
		err = x.do(ctx, op, stepCommit)
	})
	return
}

func (x *executor) run(ctx context.Context, op Operation, remoteStateChange <-chan remotestate.Snapshot) error {
	x.logger.Debugf(context.TODO(), "running operation %v for %s", op, x.unitName)

	if op.NeedsGlobalMachineLock() {
		x.logger.Debugf(context.TODO(), "acquiring machine lock for %s", x.unitName)
		releaser, err := x.acquireMachineLock(op.String(), op.ExecutionGroup())
		if err != nil {
			return errors.Annotatef(err, "acquiring %q lock for %s", op, x.unitName)
		}
		defer x.logger.Debugf(context.TODO(), "lock released for %s", x.unitName)
		defer releaser()
	} else {
		x.logger.Debugf(context.TODO(), "no machine lock needed for %s", x.unitName)
	}

	switch err := x.do(ctx, op, stepPrepare); errors.Cause(err) {
	case ErrSkipExecute:
	case nil:
		done := make(chan struct{})
		go func() {
			for {
				select {
				case rs, ok := <-remoteStateChange:
					if !ok {
						return
					}
					op.RemoteStateChanged(rs)
				case <-done:
					return
				}
			}
		}()
		if err := x.do(ctx, op, stepExecute); err != nil {
			close(done)
			return err
		}
		close(done)
	default:
		return err
	}
	return x.do(ctx, op, stepCommit)
}

func (x *executor) do(ctx context.Context, op Operation, step executorStep) (err error) {
	message := step.message(op, x.unitName)
	x.logger.Debugf(context.TODO(), message)
	newState, firstErr := step.run(op, ctx, *x.state)
	if newState != nil {
		writeErr := x.writeState(ctx, *newState)
		if firstErr == nil {
			firstErr = writeErr
		} else if writeErr != nil {
			x.logger.Errorf(context.TODO(), "after %s for %s: %v", message, x.unitName, writeErr)
		}
	}
	return errors.Annotatef(firstErr, "%s", message)
}

func (x *executor) writeState(ctx context.Context, newState State) error {
	if err := newState.Validate(); err != nil {
		return err
	}
	if x.state != nil && x.state.match(newState) {
		return nil
	}
	if err := x.stateOps.Write(ctx, &newState); err != nil {
		return errors.Annotatef(err, "writing state")
	}
	x.state = &newState
	return nil
}
