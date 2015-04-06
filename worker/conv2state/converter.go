// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package conv2state

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"

	apimachiner "github.com/juju/juju/api/machiner"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.conv2state")

// New returns a new notify watch handler that will convert the given machine &
// agent to a state server.
func New(m *apimachiner.State, agent Agent) worker.NotifyWatchHandler {
	return &converter{machiner: wrapper{m}, agent: agent}
}

// converter is a NotifyWatchHandler that converts a unit hosting machine to a
// state machine.
type converter struct {
	agent    Agent
	machiner machiner
	machine  machine
}

// Agent is an interface that can have its password set and be told to restart.
type Agent interface {
	SetPassword(string) error
	Restart() error
	Tag() names.Tag
}

// machiner is a type that creates machines from a tag.
type machiner interface {
	Machine(tag names.MachineTag) (machine, error)
}

// machine is a type that has a list of jobs and can be watched.
type machine interface {
	Jobs() (*params.JobsResult, error)
	Watch() (watcher.NotifyWatcher, error)
}

// wrapper is a wrapper around api/machiner.State to match the (local) machiner
// interface.
type wrapper struct {
	m *apimachiner.State
}

// Machines implements machiner.Machine and returns a machine from the wrapper
// api/machiner.
func (w wrapper) Machine(tag names.MachineTag) (machine, error) {
	m, err := w.m.Machine(tag)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// SetUp implements NotifyWatchHandler's SetUp method. It returns a watcher that
// checks for changes to the current machine.
func (c *converter) SetUp() (watcher.NotifyWatcher, error) {
	m, err := c.machiner.Machine(c.agent.Tag().(names.MachineTag))
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.machine = m
	return m.Watch()
}

var utils_RandomPassword = utils.RandomPassword

// Handle implements NotifyWatchHandler's Handle method.  If the change means
// that the machine is now expected to manage the environment, we change its
// password (to set its password in mongo) and restart the agent.
func (c *converter) Handle() error {
	results, err := c.machine.Jobs()
	if err != nil {
		return errors.Annotate(err, "can't get jobs for machine")
	}
	isState := false
	for _, job := range results.Jobs {
		if job.NeedsState() {
			isState = true
			break
		}
	}
	if !isState {
		return nil
	}

	// We set the password on thisfrom the API in order to get credentials into
	// mongo.
	logger.Infof("Converting this machine to a state server.")
	pw, err := utils_RandomPassword()
	if err != nil {
		return errors.Annotate(err, "error generating new password")
	}
	if err := c.agent.SetPassword(pw); err != nil {
		return errors.Annotate(err, "error setting machine password")
	}
	return errors.Trace(c.agent.Restart())
}

// TearDown implements NotifyWatchHandler's TearDown method.
func (c *converter) TearDown() error {
	return nil
}
