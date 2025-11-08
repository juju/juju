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
	domainapplicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/errors"
)

// AppFirewallerConfig is the configuration for starting a new
// [AppFirewaller].
type AppFirewallerConfig struct {
	ApplicationService ApplicationService
	Broker             CAASBroker
	PortService        PortService
	Logger             logger.Logger
}

// AppFirewaller is a single application firewaller worker ensuring the exposed
// ports of the application are refelcted in the broker.
type AppFirewaller struct {
	catacomb catacomb.Catacomb

	appUUID            application.UUID
	applicationService ApplicationService
	broker             CAASBroker
	portService        PortService

	logger logger.Logger
}

// ensureOpenPorts is responsible for making sure that the applications
// current open port requirements are reflected on the application via the
// [PortMutator]. This func returns the latest [network.GroupedPortRanges] that
// have been applied to the application.
func (w *AppFirewaller) ensureOpenPorts(
	ctx context.Context,
	mutator PortMutator,
	lastCheckPoint network.GroupedPortRanges,
) (network.GroupedPortRanges, error) {
	changedPortRanges, err := w.portService.GetApplicationOpenedPortsByEndpoint(ctx, w.appUUID)
	if err != nil {
		return nil, err
	}

	if lastCheckPoint.EqualTo(changedPortRanges) {
		w.logger.Debugf(ctx, "application %q opened ports are up to date, no work to be performed")
		return lastCheckPoint, nil
	}

	w.logger.Infof(ctx, "applying application %q updated port changes", w.appUUID)
	err = mutator.UpdatePorts(toServicePorts(changedPortRanges), false)
	if err != nil {
		return nil, errors.Errorf(
			"updating application %q ports in broker: %w", w.appUUID, err,
		)
	}

	return changedPortRanges, nil
}

// getPortMutator returns the portMutator for the current application.
func (w *AppFirewaller) getPortMutator(ctx context.Context) (PortMutator, error) {
	appName, err := w.applicationService.GetApplicationName(ctx, w.appUUID)
	if err != nil {
		return nil, errors.Errorf("getting application %q name: %w", w.appUUID, err)
	}

	app := w.broker.Application(appName, caas.DeploymentStateful)
	return app, nil
}

// Kill is part of the worker.Worker interface.
// Kill sets this [AppFirewaller] to dying with a nil error shutting down the
// main loop and any child workers.
//
// Implements the [worker.Worker] interface.
func (w *AppFirewaller) Kill() {
	w.catacomb.Kill(nil)
}

// loop is the main processing routine of the worker waiting for applications to
// be added or removed from the model. For each application in the model a child
// worker will be created to manage the open ports of the application.
//
// loop returns when the worker is placed into a dying state or an expected
// error occurs.
func (w *AppFirewaller) loop() (err error) {
	ctx := w.catacomb.Context(context.Background())
	defer func() {
		// If the application has been deleted, we can return nil.
		// If are returning because of an application not found error then
		// return a nil error as this worker has nothing more to do. The
		// application has gone away. Returning will force the catacomb to shut
		// down the watchers used.
		if errors.Is(err, domainapplicationerrors.ApplicationNotFound) {
			w.logger.Debugf(
				ctx,
				"application %q removed, shutting down caas firewaller",
				w.appUUID,
			)
			err = nil
		}
	}()

	portMutator, err := w.getPortMutator(ctx)
	if err != nil {
		return errors.Errorf(
			"getting application %q port mutator: %w",
			w.appUUID, err,
		)
	}

	// lastCheckPoint keeps track of the last known port ranges that have been
	// applied to the application.
	lastCheckPoint := network.GroupedPortRanges{}

	// Ensure the applications initial set of open port requirements are applied.
	lastCheckPoint, err = w.ensureOpenPorts(ctx, portMutator, lastCheckPoint)
	if err != nil {
		return errors.Errorf(
			"ensuring initial open port requirements for application %q are applied: %w",
			w.appUUID, err,
		)
	}

	portsWatcher, err := w.portService.WatchOpenedPortsForApplication(ctx, w.appUUID)
	if err != nil {
		return errors.Errorf("getting application %q opened ports watcher: %w",
			w.appUUID, err,
		)
	}
	if err := w.catacomb.Add(portsWatcher); err != nil {
		return errors.Errorf(
			"adding application %q opened ports watcher to catacomb: %w",
			w.appUUID, err,
		)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-portsWatcher.Changes():
			if !ok {
				return errors.New(
					"application opened ports watcher channel closed unexpectedly",
				)
			}

			w.logger.Debugf(
				ctx, "received application %q port change event", w.appUUID,
			)

			lastCheckPoint, err = w.ensureOpenPorts(ctx, portMutator, lastCheckPoint)
			if err != nil {
				return err
			}
		}
	}
}

// NewAppFirewaller constructs and runs a new [AppFirewaller].
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the configuration is invalid.
func NewAppFirewaller(
	appUUID application.UUID,
	config AppFirewallerConfig,
) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Errorf("validating configuration: %w", err)
	}

	w := &AppFirewaller{
		applicationService: config.ApplicationService,
		appUUID:            appUUID,
		broker:             config.Broker,
		portService:        config.PortService,
		logger:             config.Logger,
	}

	err := catacomb.Invoke(catacomb.Plan{
		Name: "caas-firewaller-application",
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Errorf("invoking worker in catacomb: %w", err)
	}
	return w, nil
}

// Wait is part of the worker.Worker interface.
func (w *AppFirewaller) Wait() error {
	return w.catacomb.Wait()
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

// Validate checks and confirms that this [AppFirewallerConfig] is valid by
// having set and correct values.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when a value in the configuration fails validation.
func (c *AppFirewallerConfig) Validate() error {
	if c.ApplicationService == nil {
		return errors.New("not valid nil ApplicationService").Add(
			coreerrors.NotValid,
		)
	}

	if c.Broker == nil {
		return errors.New("not valid nil Broker").Add(
			coreerrors.NotValid,
		)
	}

	if c.PortService == nil {
		return errors.New("not valid nil PortService").Add(
			coreerrors.NotValid,
		)
	}

	if c.Logger == nil {
		return errors.New("not valid nil Logger").Add(
			coreerrors.NotValid,
		)
	}

	return nil
}
