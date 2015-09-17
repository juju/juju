// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"

	"github.com/juju/errors"
	corecharm "gopkg.in/juju/charm.v5"
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
	acquireMachineLock func(string) (func() error, error)
}

// NewExecutor returns an Executor which takes its starting state from the
// supplied path, and records state changes there. If no state file exists,
// the executor's starting state will include a queued Install hook, for
// the charm identified by the supplied func.
func NewExecutor(stateFilePath string, getInstallCharm func() (*corecharm.URL, error), acquireLock func(string) (func() error, error)) (Executor, error) {
	file := NewStateFile(stateFilePath)
	state, err := file.Read()
	if err == ErrNoStateFile {
		charmURL, err := getInstallCharm()
		if err != nil {
			return nil, err
		}
		state = &State{
			Kind:     Install,
			Step:     Queued,
			CharmURL: charmURL,
		}
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
func (x *executor) Run(op Operation) (runErr error) {
	logger.Infof("running operation %v", op)

	if op.NeedsGlobalMachineLock() {
		unlock, err := x.acquireMachineLock(fmt.Sprintf("executing operation: %s", op.String()))
		if err != nil {
			return errors.Annotate(err, "could not acquire lock")
		}
		// There is nothing theoretically stopping us from unlocking
		// between execute and commit, but since we're not looking for the
		// efficiency provided by that right now, we prefer to keep the logic
		// simple. This could be changed in the future.
		defer func() {
			unlockErr := unlock()
			if unlockErr != nil {
				logger.Errorf("operation failed with error %v; error overriden by unlock failure error", runErr)
				runErr = unlockErr
			}
		}()
	}

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
	logger.Infof("skipping operation %v", op)
	return x.do(op, stepCommit)
}

func (x *executor) do(op Operation, step executorStep) (err error) {
	message := step.message(op)
	logger.Infof(message)
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
