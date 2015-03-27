// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	apiconverter "github.com/juju/juju/api/converter"
	"github.com/juju/juju/api/watcher"
)

// converter is a worker that converts a unit hosting machine to a state machine.
type converter struct {
	st     *apiconverter.State
	config agent.Config
	agent  *MachineAgent
}

func (c *converter) SetUp() (watcher.NotifyWatcher, error) {
	logger.Infof("setting up Converter watcher for %s", c.config.Tag().String())
	return c.st.WatchForJobsChanges(c.config.Tag().String())
}

func (c *converter) Handle() error {
	logger.Infof("environment for %q has been changed", c.config.Tag())

	jobs, err := c.st.Jobs(c.config.Tag().String())
	if err != nil {
		return errors.Trace(err)
	}

	for _, job := range jobs.Jobs {
		if job.NeedsState() {
			logger.Warningf("converting %q to a state server", c.config.Tag())
			pw, err := utils.RandomPassword()
			if err != nil {
				return errors.Trace(err)
			}
			c.agent.AgentConfigWriter.ChangeConfig(func(config agent.ConfigSetter) error {
				config.SetPassword(pw)
				config.SetStateAddresses([]string{"localhost:37017"})
				return nil
			})
			_, entity, err := OpenAPIState(c.config, c.agent)
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
