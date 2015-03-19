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
	st     *converter.State
	ent    Entity
	config agent.Config
}

type Entity interface {
	SetPassword(string) error
	Jobs() []multiwatcher.MachineJob
}

// NewConverter ...
func NewConverter(
	ent Entity,
	st *converter.State,
	agentConfig agent.Config,
) worker.Worker {
	return worker.NewNotifyWorker(&Converter{
		ent:    ent,
		st:     st,
		config: agentConfig,
	})
}

func (c *Converter) SetUp() (watcher.NotifyWatcher, error) {
	logger.Infof("setting up Converter watcher for %s", c.config.Tag().String())
	return c.st.WatchForJobsChanges(c.config.Tag().String())
}

func (c *Converter) Handle() error {
	logger.Infof("environment for %q has been changed", c.config.Tag())
	for _, job := range c.ent.Jobs() {
		logger.Infof("job found: %q", job)
		logger.Infof("job details: #v", job)
		if job.NeedsState() {
			logger.Infof("converting %q to a state server", c.config.Tag())
			pw, err := utils.RandomPassword()
			if err != nil {
				return errors.Trace(err)
			}
			if err := c.ent.SetPassword(pw); err != nil {
				return errors.Trace(err)
			}
			// change agentConfig too?
		}
	}
	return nil
}

func (c *Converter) TearDown() error {
	return nil
}
