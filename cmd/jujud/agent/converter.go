// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	apiconverter "github.com/juju/juju/api/converter"
	"github.com/juju/juju/api/watcher"
)

// converter is a worker that converts a unit hosting machine to a state machine.
type converter struct {
	st    *apiconverter.State
	agent *MachineAgent
}

func (c *converter) SetUp() (watcher.NotifyWatcher, error) {
	logger.Infof("setting up Converter watcher")
	return c.st.WatchForJobsChanges(c.agent.CurrentConfig().Tag().String())
}

func (c *converter) Handle() error {
	config := c.agent.CurrentConfig()
	logger.Infof("environment for %q has been changed", config.Tag())
	jobs, err := c.st.Jobs(config.Tag().String())
	if err != nil {
		return errors.Trace(err)
	}

	for _, job := range jobs.Jobs {
		if job.NeedsState() {
			logger.Warningf("converting %q to a state server", config.Tag())
			pw, err := utils.RandomPassword()
			if err != nil {
				return errors.Trace(err)
			}
			ssi, exists := config.StateServingInfo()
			if !exists {
				return errors.New("can't get state serving info from config.")
			}
			addr := fmt.Sprintf("localhost:%d", ssi.StatePort)

			c.agent.AgentConfigWriter.ChangeConfig(func(config agent.ConfigSetter) error {
				config.SetPassword(pw)
				config.SetStateAddresses([]string{addr})
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
