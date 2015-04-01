// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	apiconverter "github.com/juju/juju/api/converter"
	"github.com/juju/juju/api/watcher"
)

// converter is a NotifyWatcher that converts a unit hosting machine to a
// state machine.
type converter struct {
	st    *apiconverter.State
	agent *MachineAgent
}

// SetUp implements NotifyWatcher's SetUp method. It returns a watcher that
// checks for changes to the current machine.
func (c *converter) SetUp() (watcher.NotifyWatcher, error) {
	logger.Infof("setting up converter watcher")
	return c.st.WatchMachine(c.agent.CurrentConfig().Tag().(names.MachineTag))
}

// Handle implements NotifyWatcher's Handle method.  If the change means
// that the machine is now expected to manage the environment
func (c *converter) Handle() error {
	config := c.agent.CurrentConfig()
	tag := config.Tag().(names.MachineTag)
	jobs, err := c.st.Jobs(tag)
	if err != nil {
		logger.Errorf("Error getting jobs for tag %q: %v", tag, err)
		return errors.Trace(err)
	}
	for _, job := range jobs.Jobs {
		if job.NeedsState() {
			logger.Warningf("converting %q to a state server", config.Tag())
			pw, err := utils.RandomPassword()
			if err != nil {
				return errors.Trace(err)
			}

			c.agent.AgentConfigWriter.ChangeConfig(func(config agent.ConfigSetter) error {
				config.SetPassword(pw)
				config.SetStateAddresses([]string{"localhost:37017"})
				return nil
			})
			_, entity, err := OpenAPIState(config, c.agent)
			if err != nil {
				logger.Errorf("can't open API state: %s", errors.Details(err))
				return errors.Trace(err)
			}

			if err := entity.SetPassword(pw); err != nil {
				logger.Errorf("can't set password for machine agent: %s", errors.Details(err))
				return errors.Trace(err)
			}

			return c.agent.RestartService()
		}
	}
	return nil
}

func (c *converter) TearDown() error {
	return nil
}
