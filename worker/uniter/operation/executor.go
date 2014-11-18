// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	corecharm "gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"
)

type Executor interface {
	State() State
	Run(Operation) error
	Skip(Operation) error
}

type executorStep struct {
	verb string
	run  func(op Operation, state State) (*StateChange, error)
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
	file  *StateFile
	state *State
}

func NewExecutor(file *StateFile, getInstallCharm func() (*corecharm.URL, error)) (Executor, error) {
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
		file:  file,
		state: state,
	}, nil
}

func (x *executor) State() State {
	return *x.state
}

func (x *executor) Run(op Operation) error {
	logger.Infof("running operation %v", op)
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

func (x *executor) Skip(op Operation) error {
	logger.Infof("skipping operation %v", op)
	return x.do(op, stepCommit)
}

func (x *executor) do(op Operation, step executorStep) (err error) {
	message := step.message(op)
	logger.Infof(message)
	stateChange, firstErr := step.run(op, *x.state)
	if stateChange != nil {
		writeErr := errors.Annotatef(x.writeChange(*stateChange), "writing state")
		if firstErr == nil {
			firstErr = writeErr
		} else if writeErr != nil {
			logger.Errorf("after %s: %v", message, writeErr)
		}
	}
	return errors.Annotatef(firstErr, message)
}

func (x *executor) writeChange(change StateChange) error {
	newState := *x.state
	if (change.Kind == RunHook && change.Step == Done) || (change.Kind == Continue && change.Hook != nil) {
		switch change.Hook.Kind {
		case hooks.Start:
			newState.Started = true
		case hooks.CollectMetrics:
			newState.CollectMetricsTime = time.Now().Unix()
		}
	}
	newState.Kind = change.Kind
	newState.Step = change.Step
	newState.Hook = change.Hook
	newState.CharmURL = change.CharmURL
	newState.ActionId = change.ActionId
	if err := x.file.Write(
		newState.Started,
		newState.Kind,
		newState.Step,
		newState.Hook,
		newState.CharmURL,
		newState.ActionId,
		newState.CollectMetricsTime,
	); err != nil {
		return err
	}
	x.state = &newState
	return nil
}
