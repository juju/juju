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

func (step executorStep) message(op Operation) string {
	return fmt.Sprintf("%s operation %q", step.verb, op)
}

var (
	stepPrepare = executorStep{"preparing", Operation.Prepare}
	stepExecute = executorStep{"executing", Operation.Execute}
	stepCommit  = executorStep{"committing", Operation.Commit}
)

type executor struct {
	file               *StateFile
	state              *State
	acquireMachineLock func(string) (func(), error)
}

// NewExecutor returns an Executor which takes its starting state from the
// supplied path, and records state changes there. If no state file exists,
// the executor's starting state will include a queued Install hook, for
// the charm identified by the supplied func.
func NewExecutor(stateFilePath string, initialState State, acquireLock func(string) (func(), error)) (Executor, error) {
	file := NewStateFile(stateFilePath)
	state, err := file.Read()
	if err == ErrNoStateFile {
		state = &initialState
	} else if err != nil {
		return nil, err
	}
	return &executor{
		file:               file,
		state:              state,
		acquireMachineLock: acquireLock,
	}, nil
}

// State is part of the Executor interface.
func (x *executor) State() State {
	return *x.state
}

// Run is part of the Executor interface.
func (x *executor) Run(op Operation, remoteStateChange <-chan remotestate.Snapshot) error {
	logger.Debugf("running operation %v", op)

	if op.NeedsGlobalMachineLock() {
		releaser, err := x.acquireMachineLock(op.String())
		if err != nil {
			return errors.Annotate(err, "could not acquire lock")
		}
		defer logger.Debugf("lock released")
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
	logger.Debugf("skipping operation %v", op)
	return x.do(op, stepCommit)
}

func (x *executor) do(op Operation, step executorStep) (err error) {
	message := step.message(op)
	logger.Debugf(message)
	newState, firstErr := step.run(op, *x.state)
	if newState != nil {
		writeErr := x.writeState(*newState)
		if firstErr == nil {
			firstErr = writeErr
		} else if writeErr != nil {
			logger.Errorf("after %s: %v", message, writeErr)
		}
	}
	return errors.Annotatef(firstErr, message)
}

func (x *executor) writeState(newState State) error {
	if err := newState.validate(); err != nil {
		return err
	}
	if err := x.file.Write(&newState); err != nil {
		return errors.Annotatef(err, "writing state")
	}
	x.state = &newState
	return nil
}
