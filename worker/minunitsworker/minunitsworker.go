// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package minunitsworker

import (
	"github.com/juju/loggo"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/watcher/legacy"
)

var logger = loggo.GetLogger("juju.worker.minunitsworker")

// MinUnitsWorker ensures the minimum number of units for applications is respected.
type MinUnitsWorker struct {
	st *state.State
}

// NewMinUnitsWorker returns a Worker that runs application.EnsureMinUnits()
// if the number of alive units belonging to a application decreases, or if the
// minimum required number of units for a application is increased.
func NewMinUnitsWorker(st *state.State) worker.Worker {
	mu := &MinUnitsWorker{st: st}
	return legacy.NewStringsWorker(mu)
}

func (mu *MinUnitsWorker) SetUp() (state.StringsWatcher, error) {
	return mu.st.WatchMinUnits(), nil
}

func (mu *MinUnitsWorker) handleOneApplication(applicationName string) error {
	application, err := mu.st.Application(applicationName)
	if err != nil {
		return err
	}
	return application.EnsureMinUnits()
}

func (mu *MinUnitsWorker) Handle(applicationNames []string) error {
	for _, name := range applicationNames {
		logger.Infof("processing application %q", name)
		if err := mu.handleOneApplication(name); err != nil {
			logger.Errorf("failed to process application %q: %v", name, err)
			return err
		}
	}
	return nil
}

func (mu *MinUnitsWorker) TearDown() error {
	// Nothing to do here.
	return nil
}
