// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package converter

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/converter"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.converter")

// Converter ...
type Converter struct {
	st          *converter.State
	getEnt      func() (Entity, error)
	restart     func() error
	setPassword func(pw string)
	config      agent.Config
}

type Entity interface {
	SetPassword(string) error
	Jobs() []multiwatcher.MachineJob
}

// NewConverter ...
func NewConverter(
	getEnt func() (Entity, error),
	setPW func(pw string),
	restart func() error,
	st *converter.State,
	agentConfig agent.Config,
) worker.Worker {
	return worker.NewNotifyWorker(&Converter{
		getEnt:      getEnt,
		setPassword: setPW,
		restart:     restart,
		st:          st,
		config:      agentConfig,
	})
}

func (c *Converter) SetUp() (watcher.NotifyWatcher, error) {
	logger.Infof("setting up Converter watcher for %s", c.config.Tag().String())
	return c.st.WatchForJobsChanges(c.config.Tag().String())
}

func (c *Converter) Handle() error {
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
			ent, err := c.getEnt()
			if err != nil {
				logger.Errorf("error from getEntity: %s", errors.Details(err))
				return errors.Trace(err)
			}

			if err := ent.SetPassword(pw); err != nil {
				logger.Errorf("error trying to set password for machine agent: %s", errors.Details(err))
				return errors.Trace(err)
			}
			c.setPassword(pw)

			return c.restart()
		}
	}
	return nil
}

func (c *Converter) TearDown() error {
	return nil
}
