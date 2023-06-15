// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This worker is responsible for watching for scale changes in the number of
// units and scaling out applications. It's also responsible for reporting the
// service info (such as IP addresses) of unit pods.

package caasunitprovisioner

import (
	"sync"

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

// Config holds configuration for the CAAS unit provisioner worker.
type Config struct {
	ApplicationGetter  ApplicationGetter
	ApplicationUpdater ApplicationUpdater
	ServiceBroker      ServiceBroker

	ContainerBroker          ContainerBroker
	ProvisioningInfoGetter   ProvisioningInfoGetter
	ProvisioningStatusSetter ProvisioningStatusSetter
	LifeGetter               LifeGetter
	UnitUpdater              UnitUpdater
	CharmGetter              CharmGetter

	Logger Logger
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.ApplicationGetter == nil {
		return errors.NotValidf("missing ApplicationGetter")
	}
	if config.ApplicationUpdater == nil {
		return errors.NotValidf("missing ApplicationUpdater")
	}
	if config.ServiceBroker == nil {
		return errors.NotValidf("missing ServiceBroker")
	}
	if config.ContainerBroker == nil {
		return errors.NotValidf("missing ContainerBroker")
	}
	if config.ProvisioningInfoGetter == nil {
		return errors.NotValidf("missing ProvisioningInfoGetter")
	}
	if config.LifeGetter == nil {
		return errors.NotValidf("missing LifeGetter")
	}
	if config.UnitUpdater == nil {
		return errors.NotValidf("missing UnitUpdater")
	}
	if config.ProvisioningStatusSetter == nil {
		return errors.NotValidf("missing ProvisioningStatusSetter")
	}
	if config.CharmGetter == nil {
		return errors.NotValidf("missing CharmGetter")
	}
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

// NewWorker starts and returns a new CAAS unit provisioner worker.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	p := &provisioner{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}

type provisioner struct {
	catacomb catacomb.Catacomb
	config   Config

	// appWorkers holds the worker created to manage each application.
	// It's defined here so that we can access it in tests.
	appWorkers map[string]*applicationWorker
	mu         sync.Mutex
}

// Kill is part of the worker.Worker interface.
func (p *provisioner) Kill() {
	p.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (p *provisioner) Wait() error {
	return p.catacomb.Wait()
}

// These helper methods protect the appWorkers map so we can access for testing.

func (p *provisioner) saveApplicationWorker(appName string, aw *applicationWorker) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.appWorkers == nil {
		p.appWorkers = make(map[string]*applicationWorker)
	}
	p.appWorkers[appName] = aw
}

func (p *provisioner) deleteApplicationWorker(appName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.appWorkers, appName)
}

func (p *provisioner) getApplicationWorker(appName string) (*applicationWorker, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.appWorkers) == 0 {
		return nil, false
	}
	aw, ok := p.appWorkers[appName]
	return aw, ok
}

func (p *provisioner) loop() error {
	logger := p.config.Logger
	w, err := p.config.ApplicationGetter.WatchApplications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(w); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case apps, ok := <-w.Changes():
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
					p.config.Logger.Tracef("v1 unit provisioner got event for v2 app %q, skipping", appName)
					continue
				}

				appLife, err := p.config.LifeGetter.Life(appName)
				if err != nil && !errors.IsNotFound(err) {
					return errors.Trace(err)
				}
				if err != nil || appLife == life.Dead {
					// Once an application is deleted, remove the k8s service and ingress resources.
					if err := p.config.ServiceBroker.UnexposeService(appName); err != nil {
						return errors.Trace(err)
					}
					if err := p.config.ServiceBroker.DeleteService(appName); err != nil {
						return errors.Trace(err)
					}
					w, ok := p.getApplicationWorker(appName)
					if ok {
						// Before stopping the application worker, inform it that
						// the app is gone so it has a chance to clean up.
						// The worker will act on the removal prior to processing the
						// Stop() request.
						if err := worker.Stop(w); err != nil {
							logger.Errorf("stopping application worker for %v: %v", appName, err)
						}
						p.deleteApplicationWorker(appName)
					}
					// Start the application undertaker worker to watch the cluster
					// and wait for resources to be cleaned up.
					mode, err := p.config.ApplicationGetter.DeploymentMode(appName)
					if err != nil {
						return errors.Trace(err)
					}
					uw, err := newApplicationUndertaker(
						appName,
						mode,
						p.config.ServiceBroker,
						p.config.ContainerBroker,
						p.config.ApplicationUpdater,
						logger,
					)
					if err != nil {
						return errors.Trace(err)
					}
					_ = p.catacomb.Add(uw)
					continue
				}
				if _, ok := p.getApplicationWorker(appName); ok || appLife == life.Dead {
					// Already watching the application. or we're
					// not yet watching it and it's dead.
					continue
				}
				mode, err := p.config.ApplicationGetter.DeploymentMode(appName)
				if err != nil {
					return errors.Trace(err)
				}
				w, err := newApplicationWorker(
					appName,
					mode,
					p.config.ServiceBroker,
					p.config.ContainerBroker,
					p.config.ProvisioningStatusSetter,
					p.config.ProvisioningInfoGetter,
					p.config.ApplicationGetter,
					p.config.ApplicationUpdater,
					p.config.UnitUpdater,
					p.config.CharmGetter,
					logger,
				)
				if err != nil {
					return errors.Trace(err)
				}
				p.saveApplicationWorker(appName, w)
				_ = p.catacomb.Add(w)
			}
		}
	}
}

func (p *provisioner) charmFormat(appName string) (charm.Format, error) {
	charmInfo, err := p.config.CharmGetter.ApplicationCharmInfo(appName)
	if err != nil {
		return charm.FormatUnknown, errors.Annotatef(err, "failed to get charm info for application %q", appName)
	}
	return charm.MetaFormat(charmInfo.Charm()), nil
}
