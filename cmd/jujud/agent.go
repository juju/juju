// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/agent"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/deployer"
	"launchpad.net/tomb"
	"time"
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

type task interface {
	Stop() error
	Wait() error
	String() string
}

// runTasks runs all the given tasks until any of them fails with an
// error.  It then stops all of them and returns that error.  If a value
// is received on the stop channel, the workers are stopped.
func runTasks(stop <-chan struct{}, tasks ...task) error {
	type errInfo struct {
		index int
		err   error
	}
	logged := make(map[int]bool)
	done := make(chan errInfo, len(tasks))
	for i, t := range tasks {
		i, t := i, t
		go func() {
			done <- errInfo{i, t.Wait()}
		}()
	}
	var err error
waiting:
	for _ = range tasks {
		select {
		case info := <-done:
			if info.err != nil {
				log.Errorf("%s: %v", tasks[info.index], info.err)
				logged[info.index] = true
				err = info.err
				break waiting
			}
		case <-stop:
			break waiting
		}
	}
	// Stop all the tasks. We choose the most important error
	// to return.
	for i, t := range tasks {
		err1 := t.Stop()
		if !logged[i] && err1 != nil {
			log.Errorf("%s: %v", t, err1)
			logged[i] = true
		}
		if moreImportant(err1, err) {
			err = err1
		}
	}
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
	Tomb() *tomb.Tomb
	RunOnce(st *state.State, entity AgentState) error
	Entity(st *state.State) (AgentState, error)
	Tag() string
}

// runLoop repeatedly calls runOnce until it returns worker.ErrTerminateAgent
// or an upgraded error, or a value is received on stop.
func runLoop(runOnce func() error, stop <-chan struct{}) error {
	log.Noticef("agent starting")
	for {
		err := runOnce()
		if err == worker.ErrTerminateAgent {
			log.Noticef("entity is terminated")
			return nil
		}
		if isFatal(err) {
			return err
		}
		if err == nil {
			log.Errorf("agent died with no error")
		} else {
			log.Errorf("%v", err)
		}
		if !isleep(retryDelay, stop) {
			return nil
		}
		log.Noticef("rerunning agent")
	}
	panic("unreachable")
}

type fatalError struct {
	Err string
}

func (e *fatalError) Error() string {
	return e.Err
}

func isFatal(err error) bool {
	if err == worker.ErrTerminateAgent || isUpgraded(err) {
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

// RunAgentLoop repeatedly connects to the state server
// and calls the agent's RunOnce method.
func RunAgentLoop(c *agent.Conf, a Agent) error {
	return runLoop(func() error {
		st, entity, err := openState(c, a)
		if err != nil {
			return err
		}
		defer st.Close()
		// TODO(rog) connect to API.
		return a.RunOnce(st, entity)
	}, a.Tomb().Dying())
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

// newDeployContext gives the tests the opportunity to create a deployer.Context
// that can be used for testing so as to avoid (1) deploying units to the system
// running the tests and (2) get access to the *State used internally, so that
// tests can be run without waiting for the 5s watcher refresh time to which we would
// otherwise be restricted.
var newDeployContext = func(st *state.State, dataDir string, deployerName string) deployer.Context {
	// TODO: pick context kind based on entity name? (once we have a
	// container context for principal units, that is; for now, there
	// is no distinction between principal and subordinate deployments)
	return deployer.NewSimpleContext(dataDir, st.CACert(), deployerName, st)
}

func newDeployer(st *state.State, w *state.UnitsWatcher, dataDir string) *deployer.Deployer {
	ctx := newDeployContext(st, dataDir, w.Tag())
	return deployer.NewDeployer(st, ctx, w)
}
