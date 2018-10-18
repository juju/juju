// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/fortress"
)

var logger = loggo.GetLogger("juju.worker.migrationminion")

// Facade exposes controller functionality to a Worker.
type Facade interface {
	Watch() (watcher.MigrationStatusWatcher, error)
	Report(migrationId string, phase migration.Phase, success bool) error
}

// Config defines the operation of a Worker.
type Config struct {
	Agent             agent.Agent
	Facade            Facade
	Guard             fortress.Guard
	APIOpen           func(*api.Info, api.DialOpts) (api.Connection, error)
	ValidateMigration func(base.APICaller) error
}

// Validate returns an error if config cannot drive a Worker.
func (config Config) Validate() error {
	if config.Agent == nil {
		return errors.NotValidf("nil Agent")
	}
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Guard == nil {
		return errors.NotValidf("nil Guard")
	}
	if config.APIOpen == nil {
		return errors.NotValidf("nil APIOpen")
	}
	if config.ValidateMigration == nil {
		return errors.NotValidf("nil ValidateMigration")
	}
	return nil
}

// New returns a Worker backed by config, or an error.
func New(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &Worker{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Worker waits for a model migration to be active, then locks down the
// configured fortress and implements the migration.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill implements worker.Worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	watcher, err := w.config.Facade.Watch()
	if err != nil {
		return errors.Annotate(err, "setting up watcher")
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case status, ok := <-watcher.Changes():
			if !ok {
				return errors.New("watcher channel closed")
			}
			if err := w.handle(status); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (w *Worker) handle(status watcher.MigrationStatus) error {
	logger.Infof("migration phase is now: %s", status.Phase)

	if !status.Phase.IsRunning() {
		return w.config.Guard.Unlock()
	}

	// Ensure that all workers related to migration fortress have
	// stopped and aren't allowed to restart.
	err := w.config.Guard.Lockdown(w.catacomb.Dying())
	if errors.Cause(err) == fortress.ErrAborted {
		return w.catacomb.ErrDying()
	} else if err != nil {
		return errors.Trace(err)
	}

	switch status.Phase {
	case migration.QUIESCE:
		err = w.doQUIESCE(status)
	case migration.VALIDATION:
		err = w.doVALIDATION(status)
	case migration.SUCCESS:
		err = w.doSUCCESS(status)
	default:
		// The minion doesn't need to do anything for other
		// migration phases.
	}
	return errors.Trace(err)
}

func (w *Worker) doQUIESCE(status watcher.MigrationStatus) error {
	// Report that the minion is ready and that all workers that
	// should be shut down have done so.
	return w.report(status, true)
}

func (w *Worker) doVALIDATION(status watcher.MigrationStatus) error {
	err := w.validate(status)
	if err != nil {
		// Don't return this error just log it and report to the
		// migrationmaster that things didn't work out.
		logger.Errorf("validation failed: %v", err)
	}
	return w.report(status, err == nil)
}

func (w *Worker) validate(status watcher.MigrationStatus) error {
	agentConf := w.config.Agent.CurrentConfig()
	apiInfo, ok := agentConf.APIInfo()
	if !ok {
		return errors.New("no API connection details")
	}
	apiInfo.Addrs = status.TargetAPIAddrs
	apiInfo.CACert = status.TargetCACert
	// Application agents (k8s) use old password.
	if apiInfo.Password == "" {
		apiInfo.Password = agentConf.OldPassword()
	}

	// Use zero DialOpts (no retries) because the worker must stay
	// responsive to Kill requests. We don't want it to be blocked by
	// a long set of retry attempts.
	conn, err := w.config.APIOpen(apiInfo, api.DialOpts{})
	if err != nil {
		// Don't return this error just log it and report to the
		// migrationmaster that things didn't work out.
		return errors.Annotate(err, "failed to open API to target controller")
	}
	defer conn.Close()

	// Ask the agent to confirm that things look ok.
	err = w.config.ValidateMigration(conn)
	return errors.Trace(err)
}

func (w *Worker) doSUCCESS(status watcher.MigrationStatus) error {
	hps, err := apiAddrsToHostPorts(status.TargetAPIAddrs)
	if err != nil {
		return errors.Annotate(err, "converting API addresses")
	}

	// Report first because the config update that's about to happen
	// will cause the API connection to drop. The SUCCESS phase is the
	// point of no return anyway.
	if err := w.report(status, true); err != nil {
		return errors.Trace(err)
	}

	err = w.config.Agent.ChangeConfig(func(conf agent.ConfigSetter) error {
		conf.SetAPIHostPorts(hps)
		conf.SetCACert(status.TargetCACert)
		return nil
	})
	return errors.Annotate(err, "setting agent config")
}

func (w *Worker) report(status watcher.MigrationStatus, success bool) error {
	logger.Debugf("reporting back for phase %s: %v", status.Phase, success)
	err := w.config.Facade.Report(status.MigrationId, status.Phase, success)
	return errors.Annotate(err, "failed to report phase progress")
}

func apiAddrsToHostPorts(addrs []string) ([][]network.HostPort, error) {
	hps, err := network.ParseHostPorts(addrs...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return [][]network.HostPort{hps}, nil
}
