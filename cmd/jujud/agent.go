// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io"
	"time"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apiagent "launchpad.net/juju-core/state/api/agent"
	apideployer "launchpad.net/juju-core/state/api/deployer"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/deployer"
)

// requiredError is useful when complaining about missing command-line options.
func requiredError(name string) error {
	return fmt.Errorf("--%s option must be set", name)
}

// AgentConf handles command-line flags shared by all agents.
type AgentConf struct {
	*agent.Conf
	dataDir string
}

// addFlags injects common agent flags into f.
func (c *AgentConf) addFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.dataDir, "data-dir", "/var/lib/juju", "directory for juju data")
}

func (c *AgentConf) checkArgs(args []string) error {
	if c.dataDir == "" {
		return requiredError("data-dir")
	}
	return cmd.CheckEmpty(args)
}

func (c *AgentConf) read(tag string) error {
	var err error
	c.Conf, err = agent.ReadConf(c.dataDir, tag)
	return err
}

func importance(err error) int {
	switch {
	case err == nil:
		return 0
	default:
		return 1
	case isUpgraded(err):
		return 2
	case err == worker.ErrTerminateAgent:
		return 3
	}
	panic("unreachable")
}

// moreImportant returns whether err0 is
// more important than err1 - that is, whether
// we should act on err0 in preference to err1.
func moreImportant(err0, err1 error) bool {
	return importance(err0) > importance(err1)
}

func isUpgraded(err error) bool {
	_, ok := err.(*UpgradeReadyError)
	return ok
}

type Agent interface {
	Entity(st *state.State) (AgentState, error)
	Tag() string
}

// The AgentState interface is implemented by state types
// that represent running agents.
type AgentState interface {
	// SetAgentTools sets the tools that the agent is currently running.
	SetAgentTools(tools *tools.Tools) error
	Tag() string
	SetMongoPassword(password string) error
	Life() state.Life
}

type fatalError struct {
	Err string
}

func (e *fatalError) Error() string {
	return e.Err
}

func isFatal(err error) bool {
	isTerminate := err == worker.ErrTerminateAgent
	notProvisioned := params.ErrCode(err) == params.CodeNotProvisioned
	if isTerminate || notProvisioned || isUpgraded(err) {
		return true
	}
	_, ok := err.(*fatalError)
	return ok
}

// isleep waits for the given duration or until it receives a value on
// stop.  It returns whether the full duration was slept without being
// stopped.
func isleep(d time.Duration, stop <-chan struct{}) bool {
	select {
	case <-stop:
		return false
	case <-time.After(d):
	}
	return true
}

func openState(c *agent.Conf, a Agent) (*state.State, AgentState, error) {
	st, err := c.OpenState()
	if err != nil {
		return nil, nil, err
	}
	entity, err := a.Entity(st)
	if errors.IsNotFoundError(err) || err == nil && entity.Life() == state.Dead {
		err = worker.ErrTerminateAgent
	}
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	return st, entity, nil
}

func openAPIState(c *agent.Conf, a Agent) (*api.State, *apiagent.Entity, error) {
	// We let the API dial fail immediately because the
	// runner's loop outside the caller of openAPIState will
	// keep on retrying. If we block for ages here,
	// then the worker that's calling this cannot
	// be interrupted.
	st, newPassword, err := c.OpenAPI(api.DialOpts{})
	if err != nil {
		return nil, nil, err
	}
	entity, err := st.Agent().Entity(a.Tag())
	if params.ErrCode(err) == params.CodeNotFound || err == nil && entity.Life() == params.Dead {
		err = worker.ErrTerminateAgent
	}
	if err != nil {
		st.Close()
		return nil, nil, err
	}
	if newPassword == "" {
		return st, entity, nil
	}
	// Make a copy of the configuration so that if we fail
	// to write the configuration file, the configuration will
	// still be valid.
	c1 := *c
	stateInfo := *c.StateInfo
	c1.StateInfo = &stateInfo
	apiInfo := *c.APIInfo
	c1.APIInfo = &apiInfo

	c1.StateInfo.Password = newPassword
	c1.APIInfo.Password = newPassword
	if err := c1.Write(); err != nil {
		return nil, nil, err
	}
	*c = c1
	if err := entity.SetPassword(newPassword); err != nil {
		return nil, nil, err
	}
	return st, entity, nil

}

// agentDone processes the error returned by
// an exiting agent.
func agentDone(err error) error {
	if err == worker.ErrTerminateAgent {
		err = nil
	}
	if ug, ok := err.(*UpgradeReadyError); ok {
		if err := ug.ChangeAgentTools(); err != nil {
			// Return and let upstart deal with the restart.
			return err
		}
	}
	return err
}

type closeWorker struct {
	worker worker.Worker
	closer io.Closer
}

// newCloseWorker returns a task that wraps the given task,
// closing the given closer when it finishes.
func newCloseWorker(worker worker.Worker, closer io.Closer) worker.Worker {
	return &closeWorker{
		worker: worker,
		closer: closer,
	}
}

func (c *closeWorker) Kill() {
	c.worker.Kill()
}

func (c *closeWorker) Wait() error {
	err := c.worker.Wait()
	if err := c.closer.Close(); err != nil {
		log.Errorf("closeWorker: close error: %v", err)
	}
	return err
}

// newDeployContext gives the tests the opportunity to create a deployer.Context
// that can be used for testing so as to avoid (1) deploying units to the system
// running the tests and (2) get access to the *State used internally, so that
// tests can be run without waiting for the 5s watcher refresh time to which we would
// otherwise be restricted.
var newDeployContext = func(st *apideployer.State, dataDir string) (deployer.Context, error) {
	caCert, err := st.CACert()
	if err != nil {
		return nil, err
	}
	return deployer.NewSimpleContext(dataDir, caCert, st), nil
}

func newDeployer(st *apideployer.State, machineTag string, dataDir string) (worker.Worker, error) {
	ctx, err := newDeployContext(st, dataDir)
	if err != nil {
		return nil, err
	}
	return deployer.NewDeployer(st, ctx, machineTag), nil
}
