// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/life"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// Config holds configuration for the CAAS unit firewaller worker.
type Config struct {
	ControllerUUID    string
	ModelUUID         string
	ApplicationGetter ApplicationGetter
	LifeGetter        LifeGetter
	CharmGetter       CharmGetter
	ServiceExposer    ServiceExposer
	Logger            Logger
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.ControllerUUID == "" {
		return errors.NotValidf("missing ControllerUUID")
	}
	if config.ModelUUID == "" {
		return errors.NotValidf("missing ModelUUID")
	}
	if config.ApplicationGetter == nil {
		return errors.NotValidf("missing ApplicationGetter")
	}
	if config.LifeGetter == nil {
		return errors.NotValidf("missing LifeGetter")
	}
	if config.CharmGetter == nil {
		return errors.NotValidf("missing CharmGetter")
	}
	if config.ServiceExposer == nil {
		return errors.NotValidf("missing ServiceExposer")
	}
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

// NewWorker starts and returns a new CAAS unit firewaller worker.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	p := &firewaller{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}

type firewaller struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill is part of the worker.Worker interface.
func (p *firewaller) Kill() {
	p.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (p *firewaller) Wait() error {
	return p.catacomb.Wait()
}

func (p *firewaller) loop() error {
	logger := p.config.Logger
	appWatcher, err := p.config.ApplicationGetter.WatchApplications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}

	appWorkers := make(map[string]worker.Worker)
	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case apps, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			for _, appName := range apps {
				// If charm is (now) a v2 charm, skip processing.
				format, err := p.charmFormat(appName)
				if errors.IsNotFound(err) {
					p.config.Logger.Debugf("application %q no longer exists", appName)
					continue
				} else if err != nil {
					return errors.Trace(err)
				}
				if format >= charm.FormatV2 {
					p.config.Logger.Tracef("v1 caasfirewaller got event for v2 app %q, skipping", appName)
					continue
				}

				appLife, err := p.config.LifeGetter.Life(appName)
				if errors.IsNotFound(err) || appLife == life.Dead {
					if appWorker, ok := appWorkers[appName]; ok {
						if err := worker.Stop(appWorker); err != nil {
							logger.Errorf("error stopping caas firewaller: %v", err)
						}
						delete(appWorkers, appName)
					}
					continue
				}
				if err != nil {
					return errors.Trace(err)
				}
				if _, ok := appWorkers[appName]; ok {
					// Already watching the application.
					continue
				}
				appWorker, err := newApplicationWorker(
					p.config.ControllerUUID,
					p.config.ModelUUID,
					appName,
					p.config.ApplicationGetter,
					p.config.ServiceExposer,
					p.config.LifeGetter,
					p.config.CharmGetter,
					logger,
				)
				if err != nil {
					return errors.Trace(err)
				}
				appWorkers[appName] = appWorker
				_ = p.catacomb.Add(appWorker)
			}
		}
	}
}

func (p *firewaller) charmFormat(appName string) (charm.Format, error) {
	charmInfo, err := p.config.CharmGetter.ApplicationCharmInfo(appName)
	if err != nil {
		return charm.FormatUnknown, errors.Annotatef(err, "failed to get charm info for application %q", appName)
	}
	return charm.MetaFormat(charmInfo.Charm()), nil
}
