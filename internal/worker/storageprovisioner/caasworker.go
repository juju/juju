// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// ApplicationWatcher provides an interface for
// watching for the lifecycle state changes
// (including addition) of applications.
type ApplicationWatcher interface {
	WatchApplications(context.Context) (watcher.StringsWatcher, error)
}

// NewCaasWorker starts and returns a new CAAS storage provisioner worker.
// The worker provisions model scoped storage and also watches and starts
// provisioner workers to handle storage for application units.
func NewCaasWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	p := &provisioner{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "storage-provisioner",
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}

type provisioner struct {
	catacomb catacomb.Catacomb
	config   Config

	// provisioners holds the worker created to manage each application.
	// It's defined here so that we can access it in tests.
	provisioners map[string]worker.Worker
	mu           sync.Mutex
}

// Kill is part of the worker.Worker interface.
func (p *provisioner) Kill() {
	p.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (p *provisioner) Wait() error {
	return p.catacomb.Wait()
}

// These helper methods protect the provisioners map so we can access for testing.

func (p *provisioner) saveApplicationWorker(appName string, aw worker.Worker) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.provisioners == nil {
		p.provisioners = make(map[string]worker.Worker)
	}
	p.provisioners[appName] = aw
}

func (p *provisioner) deleteApplicationWorker(appName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.provisioners, appName)
}

func (p *provisioner) getApplicationWorker(appName string) (worker.Worker, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.provisioners) == 0 {
		return nil, false
	}
	aw, ok := p.provisioners[appName]
	return aw, ok
}

func (p *provisioner) loop() error {
	ctx, cancel := p.scopedContext()
	defer cancel()

	appsWatcher, err := p.config.Applications.WatchApplications(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(appsWatcher); err != nil {
		return errors.Trace(err)
	}

	modelProvisioner, err := NewStorageProvisioner(p.config)
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(modelProvisioner); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case apps, ok := <-appsWatcher.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			appTags := make([]names.Tag, len(apps))
			for i, appID := range apps {
				appTags[i] = names.NewApplicationTag(appID)
			}
			appsLife, err := p.config.Life.Life(ctx, appTags)
			if err != nil {
				return errors.Annotate(err, "getting application life")
			}
			for i, appID := range apps {
				appLifeResult := appsLife[i]
				if appLifeResult.Error != nil && params.IsCodeNotFound(appLifeResult.Error) || appLifeResult.Life == life.Dead {
					p.config.Logger.Debugf(ctx, "app %v not found", appID)
					if appWorker, ok := p.getApplicationWorker(appID); ok {
						if err := worker.Stop(appWorker); err != nil {
							p.config.Logger.Errorf(ctx, "stopping application storage worker for %v: %v", appID, err)
						}
						p.deleteApplicationWorker(appID)
					}
					continue
				}
				if _, ok := p.getApplicationWorker(appID); ok {
					// Already watching the application.
					continue
				}
				cfg := p.config
				cfg.Scope = appTags[i]
				w, err := NewStorageProvisioner(cfg)
				if err != nil {
					return errors.Trace(err)
				}
				p.saveApplicationWorker(appID, w)
				_ = p.catacomb.Add(w)
			}
		}
	}
}

func (p *provisioner) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(p.catacomb.Context(context.Background()))
}
