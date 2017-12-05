// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE store for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/mutex"
)

type executorStep struct {
	verb string
	run  func(op Operation, state State) (*State, error)
}

func (step executorStep) message(op Operation) string {
	return fmt.Sprintf("%s operation %q", step.verb, op)
}

var (
	stepPrepare = executorStep{"preparing", Operation.Prepare}
	stepExecute = executorStep{"executing", Operation.Execute}
	stepCommit  = executorStep{"committing", Operation.Commit}
)

type executor struct {
	state              *State
	acquireMachineLock func() (mutex.Releaser, error)
}

// NewExecutor returns an Executor which takes its starting state from the
// supplied path, and records state changes there. If no state store exists,
// the executor's starting state will include a queued Install hook, for
// the charm identified by the supplied func.
func NewExecutor() (Executor, error) {
	state := &State{
		Kind: Continue,
		Step: Pending,
	}
	return &executor{
		state: state,
	}, nil
}

// State is part of the Executor interface.
func (x *executor) State() State {
	return *x.state
}

// Run is part of the Executor interface.
func (x *executor) Run(op Operation) error {
	logger.Debugf("running operation %v", op)

	switch err := x.do(op, stepPrepare); errors.Cause(err) {
	case ErrSkipExecute:
	case nil:
		if err := x.do(op, stepExecute); err != nil {
			return err
		}
	default:
		return err
	}
	return x.do(op, stepCommit)
}

// Skip is part of the Executor interface.
func (x *executor) Skip(op Operation) error {
	logger.Debugf("skipping operation %v", op)
	return x.do(op, stepCommit)
}

func (x *executor) do(op Operation, step executorStep) (err error) {
	message := step.message(op)
	logger.Debugf(message)
	newState, firstErr := step.run(op, *x.state)
	if newState != nil {
		writeErr := x.updateState(*newState)
		if firstErr == nil {
			firstErr = writeErr
		} else if writeErr != nil {
			logger.Errorf("after %s: %v", message, writeErr)
		}
	}
	return errors.Annotatef(firstErr, message)
}

func (x *executor) updateState(newState State) error {
	if err := newState.validate(); err != nil {
		return err
	}
	x.state = &newState
	return nil
}
