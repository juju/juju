// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"context"
	"strings"

	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
)

type applicationWorker struct {
	catacomb catacomb.Catacomb
	appName  string
	appUUID  application.UUID

	portService        PortService
	applicationService ApplicationService

	broker         CAASBroker
	portMutator    PortMutator
	serviceUpdater ServiceUpdater

	portsWatcher watcher.NotifyWatcher

	currentPorts network.GroupedPortRanges

	logger logger.Logger
}

func newApplicationWorker(
	appUUID application.UUID,
	portService PortService,
	applicationService ApplicationService,
	broker CAASBroker,
	logger logger.Logger,
) (worker.Worker, error) {
	w := &applicationWorker{
		appUUID:            appUUID,
		portService:        portService,
		applicationService: applicationService,
		broker:             broker,
		logger:             logger,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "caas-firewaller-application",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Errorf("invoking worker in catacomb: %w", err)
	}
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *applicationWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *applicationWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *applicationWorker) setUp(ctx context.Context) (err error) {
	w.appName, err = w.applicationService.GetApplicationName(ctx, w.appUUID)
	if err != nil {
		return errors.Errorf("getting application %q name: %w", w.appUUID, err)
	}
	w.portsWatcher, err = w.portService.WatchOpenedPortsForApplication(ctx, w.appUUID)
	if err != nil {
		return errors.Errorf("getting application %q opened ports watcher: %w",
			w.appUUID, err,
		)
	}
	if err := w.catacomb.Add(w.portsWatcher); err != nil {
		return errors.Errorf(
			"adding application %q opened ports watcher to catacomb: %w",
			w.appUUID, err,
		)
	}

	// TODO(sidecar): support deployment other than statefulset
	app := w.broker.Application(w.appName, caas.DeploymentStateful)
	w.portMutator = app
	w.serviceUpdater = app

	w.currentPorts, err = w.portService.GetApplicationOpenedPortsByEndpoint(
		ctx, w.appUUID,
	)
	if err != nil {
		return errors.Errorf(
			"getting application %q opened ports: %w",
			w.appUUID, err,
		)
	}
	return nil
}

func (w *applicationWorker) loop() (err error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	defer func() {
		// If the application has been deleted, we can return nil.
		if errors.Is(err, coreerrors.NotFound) {
			w.logger.Debugf(ctx, "sidecar caas firewaller application %v has been removed", w.appName)
			err = nil
		}
	}()

	if err = w.setUp(ctx); err != nil {
		return errors.Errorf("setting up initial watcher state: %w", err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-w.portsWatcher.Changes():
			if !ok {
				return errors.New(
					"application opened ports watcher channel closed unexpectedly",
				)
			}
			if err := w.onPortChanged(ctx); err != nil {
				return errors.Capture(err)
			}
		}
	}
}

func toServicePorts(in network.GroupedPortRanges) []caas.ServicePort {
	ports := in.UniquePortRanges()
	network.SortPortRanges(ports)
	out := make([]caas.ServicePort, len(ports))
	for i, p := range ports {
		out[i] = caas.ServicePort{
			// k8s complains about `/`.
			Name:       strings.Replace(p.String(), "/", "-", -1),
			Port:       p.FromPort,
			TargetPort: p.ToPort,
			Protocol:   p.Protocol,
		}
	}
	return out
}

func (w *applicationWorker) onPortChanged(ctx context.Context) error {
	changedPortRanges, err := w.portService.GetApplicationOpenedPortsByEndpoint(ctx, w.appUUID)
	if err != nil {
		return err
	}
	w.logger.Tracef(ctx, "current port for app %q, %v", w.appName, w.currentPorts)
	w.logger.Tracef(ctx, "port changed for app %q, %v", w.appName, changedPortRanges)
	if w.currentPorts.EqualTo(changedPortRanges) {
		w.logger.Debugf(ctx, "no port changes for app %q", w.appName)
		return nil
	}

	err = w.portMutator.UpdatePorts(toServicePorts(changedPortRanges), false)
	if errors.Is(err, coreerrors.NotFound) {
		return nil
	}
	if err != nil {
		return errors.Errorf(
			"updating application %q ports in broker: %w", w.appUUID, err,
		)
	}

	w.currentPorts = changedPortRanges
	return nil
}

func (w *applicationWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
