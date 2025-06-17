// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/remotestate"
)

type executorStep struct {
	verb string
	run  func(op Operation, state State) (*State, error)
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
	acquireMachineLock func(string) (func(), error)
	logger             Logger
}

// ExecutorConfig defines configuration for an Executor.
type ExecutorConfig struct {
	StateReadWriter UnitStateReadWriter
	InitialState    State
	AcquireLock     func(string) (func(), error)
	Logger          Logger
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
func NewExecutor(unitName string, cfg ExecutorConfig) (Executor, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	stateOps := NewStateOps(cfg.StateReadWriter)
	state, err := stateOps.Read()
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
func (x *executor) Run(op Operation, remoteStateChange <-chan remotestate.Snapshot) error {
	x.logger.Debugf("running operation %v for %s", op, x.unitName)

	if op.NeedsGlobalMachineLock() {
		releaser, err := x.acquireMachineLock(op.String())
		if err != nil {
			return errors.Annotatef(err, "acquiring %q lock for %s", op, x.unitName)
		}
		defer x.logger.Debugf("lock released for %s", x.unitName)
		defer releaser()
	}

	switch err := x.do(op, stepPrepare); errors.Cause(err) {
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
		if err := x.do(op, stepExecute); err != nil {
			close(done)
			return err
		}
		close(done)
	default:
		return err
	}
	return x.do(op, stepCommit)
}

// Skip is part of the Executor interface.
func (x *executor) Skip(op Operation) error {
	x.logger.Debugf("skipping operation %v for %s", op, x.unitName)
	return x.do(op, stepCommit)
}

func (x *executor) do(op Operation, step executorStep) (err error) {
	message := step.message(op, x.unitName)
	x.logger.Debugf(message)
	newState, firstErr := step.run(op, *x.state)
	if newState != nil {
		writeErr := x.writeState(*newState)
		if firstErr == nil {
			firstErr = writeErr
		} else if writeErr != nil {
			x.logger.Errorf("after %s for %s: %v", message, x.unitName, writeErr)
		}
	}
	return errors.Annotate(firstErr, message)
}

func (x *executor) writeState(newState State) error {
	if err := newState.Validate(); err != nil {
		return err
	}
	if x.state != nil && x.state.match(newState) {
		return nil
	}
	if err := x.stateOps.Write(&newState); err != nil {
		return errors.Annotatef(err, "writing state")
	}
	x.state = &newState
	return nil
}
