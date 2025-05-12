// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddresssetter

import (
	"context"

	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/errors"
)

// controllerTracker is a worker which reports changes in the addresses of a
// single controller node.
type controllerTracker struct {
	catacomb           catacomb.Catacomb
	notifyCh           chan<- struct{}
	applicationService ApplicationService
	controllerUnitName unit.Name

	logger logger.Logger
}

func newControllerTracker(unitName unit.Name, applicationService ApplicationService, notifyCh chan<- struct{}, logger logger.Logger) (*controllerTracker, error) {
	c := &controllerTracker{
		notifyCh:           notifyCh,
		controllerUnitName: unitName,
		applicationService: applicationService,
		logger:             logger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "controllertracker",
		Site: &c.catacomb,
		Work: c.loop,
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return c, nil
}

// Kill implements Worker.
func (c *controllerTracker) Kill() {
	c.catacomb.Kill(nil)
}

// Wait implements Worker.
func (c *controllerTracker) Wait() error {
	return c.catacomb.Wait()
}

func (c *controllerTracker) loop() error {
	ctx, cancel := c.scopedContext()
	defer cancel()

	netNodes, err := c.applicationService.GetUnitNetNodes(ctx, c.controllerUnitName)
	if err != nil {
		return errors.Errorf("getting net nodes for controller %q: %w", c.controllerUnitName, err)
	}
	addressWatcher, err := c.applicationService.WatchNetNodeAddress(ctx, netNodes...)
	if err != nil {
		return errors.Errorf("starting watcher for net nodes %v: %w", netNodes, err)
	}

	if err := c.catacomb.Add(addressWatcher); err != nil {
		return errors.Capture(err)
	}

	var notifyCh chan<- struct{}
	for {
		c.logger.Tracef(ctx, "waiting for addresses changes")
		select {
		case <-c.catacomb.Dying():
			return c.catacomb.ErrDying()
		case _, ok := <-addressWatcher.Changes():
			if !ok {
				return errors.Errorf("watcher for net nodes %+v closed", netNodes)
			}
			c.logger.Tracef(ctx, "<-netNodeAddressChanges")
			notifyCh = c.notifyCh
		case notifyCh <- struct{}{}:
			c.logger.Tracef(ctx, "<-notifyCh")
			notifyCh = nil
		}
	}
}

func (w *controllerTracker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
