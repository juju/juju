// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitsmanager

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	message "github.com/juju/juju/internal/pubsub/agent"
)

// Hub is a pubsub hub used for internal messaging.
type Hub interface {
	Publish(topic string, data interface{}) func()
	Subscribe(topic string, handler func(string, interface{})) func()
}

type manager struct {
	catacomb catacomb.Catacomb

	logger Logger
	clock  clock.Clock

	hub   Hub
	unsub func()
}

type Config struct {
	Logger
	Clock clock.Clock

	Hub
}

// NewWorker returns a worker that runs on CAAS agent and subscribes and handles unit topics.
func NewWorker(config Config) (worker.Worker, error) {
	w := manager{
		logger: config.Logger,
		clock:  config.Clock,
		hub:    config.Hub,
	}
	unsubStop := w.hub.Subscribe(message.StopUnitTopic, w.stopUnitRequest)
	unsubStart := w.hub.Subscribe(message.StartUnitTopic, w.startUnitRequest)
	unsubStatus := w.hub.Subscribe(message.UnitStatusTopic, w.unitStatusRequest)
	w.unsub = func() {
		unsubStop()
		unsubStart()
		unsubStatus()
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &w, nil
}

func (w *manager) stopUnitRequest(topic string, data interface{}) {
	units, ok := data.(message.Units)
	if !ok {
		w.logger.Errorf("data should be a Units structure")
	}
	response := message.StartStopResponse{
		"error": errors.NotSupportedf("stop units for %v", units).Error(),
	}
	w.hub.Publish(message.StopUnitResponseTopic, response)
}

func (w *manager) startUnitRequest(topic string, data interface{}) {
	units, ok := data.(message.Units)
	if !ok {
		w.logger.Errorf("data should be a Units structure")
	}
	response := message.StartStopResponse{
		"error": errors.NotSupportedf("start units for %v", units).Error(),
	}
	w.hub.Publish(message.StartUnitResponseTopic, response)
}

func (w *manager) unitStatusRequest(topic string, _ interface{}) {
	response := message.Status{
		"error": errors.NotSupportedf("units status").Error(),
	}
	w.hub.Publish(message.UnitStatusResponseTopic, response)
}

func (w *manager) Kill() {
	w.unsub()
	w.catacomb.Kill(nil)
}

func (w *manager) Wait() error {
	return w.catacomb.Wait()
}

func (w *manager) loop() error {
	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}
