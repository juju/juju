// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitinit

import (
	"io/ioutil"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/worker/caasoperator"
)

type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
}

// Client provides an interface for interacting
// with the CAASOperator API. Subsets of this
// should be passed to the CAASUnitInit worker.
type Client interface {
	ContainerStartWatcher
}

// ContainerStartWatcher provides an interface for watching
// for unit container starts.
type ContainerStartWatcher interface {
	WatchContainerStart(string, string) (watcher.StringsWatcher, error)
}

// Config for a caasUnitInitWorker and unitInitializer
type Config struct {
	// Logger for the worker.
	Logger Logger

	// Clock holds the clock to be used by the CAAS operator
	// for time-related operations.
	Clock clock.Clock

	// Application holds the name of the application that
	// this CAAS operator manages.
	Application string

	// DataDir holds the path to the Juju "data directory",
	// i.e. "/var/lib/juju" (by default). The CAAS operator
	// expects to find the jujud binary at <data-dir>/tools/jujud.
	DataDir string

	// ContainerStartWatcher provides an interface for watching
	// for unit container starts.
	ContainerStartWatcher ContainerStartWatcher

	// UnitProviderIDFunc returns the ProviderID for the given unit.
	UnitProviderIDFunc func(unit names.UnitTag) (string, error)

	// Paths provides CAAS operator paths.
	Paths caasoperator.Paths

	// OperatorInfo contains serving information such as Certs and PrivateKeys.
	OperatorInfo caas.OperatorInfo

	// NewExecClient for initilizing units.
	NewExecClient func() (exec.Executor, error)

	// InitializeUnit with the charm and configuration.
	InitializeUnit InitializeUnitFunc
}

func (config Config) Validate() error {
	if !names.IsValidApplication(config.Application) {
		return errors.NotValidf("application name %q", config.Application)
	}
	if config.ContainerStartWatcher == nil {
		return errors.NotValidf("missing ContainerStartWatcher")
	}
	if config.UnitProviderIDFunc == nil {
		return errors.NotValidf("missing UnitProviderIDFunc")
	}
	if config.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if config.DataDir == "" {
		return errors.NotValidf("missing DataDir")
	}
	if config.NewExecClient == nil {
		return errors.NotValidf("missing NewExecClient")
	}
	if config.InitializeUnit == nil {
		return errors.NotValidf("missing InitializeUnit")
	}
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

// InitializeUnitFunc returns a new worker start function to initilize the Unit.
type InitializeUnitFunc func(params InitializeUnitParams, cancel <-chan struct{}) error

type caasUnitInitWorker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// NewWorker returns a new CAAS unit init worker.
func NewWorker(config Config) (worker.Worker, error) {
	w := &caasUnitInitWorker{
		config: config,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (w *caasUnitInitWorker) Kill() {
	w.catacomb.Kill(nil)
}

func (w *caasUnitInitWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *caasUnitInitWorker) loop() error {
	execClient, err := w.config.NewExecClient()
	if err != nil {
		return errors.Annotatef(err, "failed to create ExecClient")
	}

	containerStartWatcher, err := w.config.ContainerStartWatcher.WatchContainerStart(w.config.Application, caas.InitContainerName)
	if err != nil {
		return errors.Annotatef(err, "failed to create container start watcher")
	}
	if err := w.catacomb.Add(containerStartWatcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case units, ok := <-containerStartWatcher.Changes():
			if !ok {
				return errors.Errorf("watcher closed channel")
			}
			for _, unit := range units {
				params := InitializeUnitParams{
					UnitTag:            names.NewUnitTag(unit),
					Logger:             w.config.Logger,
					UnitProviderIDFunc: w.config.UnitProviderIDFunc,
					Paths:              w.config.Paths,
					OperatorInfo:       w.config.OperatorInfo,
					ExecClient:         execClient,
					WriteFile:          ioutil.WriteFile,
					TempDir:            ioutil.TempDir,
				}
				err = w.config.InitializeUnit(params, w.catacomb.Dying())
				if errors.IsNotFound(err) {
					w.config.Logger.Infof("unit %q went away, skipping initialization", unit)
				} else if err != nil {
					return errors.Annotatef(err, "initializing unit %q", unit)
				}
			}
		}
	}
}
