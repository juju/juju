// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package converter

import (
	"github.com/juju/loggo"
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/converter"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.converter")

// Converter ...
type Converter struct {
	tomb tomb.Tomb
	st   *converter.State
	tag  names.Tag
}

type converterState interface {
	WatchForJobsChanges(names.MachineTag) (watcher.NotifyWatcher, error)
}

// NewConverter ...
func NewConverter(
	st *converter.State,
	agentConfig agent.Config,
) worker.Worker {
	return worker.NewNotifyWorker(&Converter{
		st:  st,
		tag: agentConfig.Tag(),
	})
}

func (c *Converter) SetUp() (watcher.NotifyWatcher, error) {
	logger.Infof("Setting up Converter watcher.")
	return c.st.WatchForJobsChanges(c.tag.String())
}

func (c *Converter) Handle() (err error) {
	logger.Infof("Jobs for %q have been changed. Check for ManageJob. Start conversion.", c.tag.String())
	return nil
}

func (c *Converter) TearDown() error {
	return nil
}
