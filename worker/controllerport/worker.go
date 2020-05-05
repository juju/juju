// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerport

import (
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/pubsub/controller"
)

// Config is the configuration required for running an API server worker.
type Config struct {
	AgentConfig             agent.Config
	Hub                     *pubsub.StructuredHub
	Logger                  Logger
	ControllerAPIPort       int
	UpdateControllerAPIPort func(int) error
}

// Validate validates the API server configuration.
func (config Config) Validate() error {
	if config.AgentConfig == nil {
		return errors.NotValidf("nil AgentConfig")
	}
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.UpdateControllerAPIPort == nil {
		return errors.NotValidf("nil UpdateControllerAPIPort")
	}
	return nil
}

// NewWorker returns a new API server worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		logger: config.Logger,
		update: config.UpdateControllerAPIPort,
	}

	servingInfo, ok := config.AgentConfig.StateServingInfo()
	if !ok {
		return nil, errors.New("missing state serving info")
	}
	w.controllerAPIPort = servingInfo.ControllerAPIPort
	// We need to make sure that update the agent config with
	// the potentially new controller api port from the database
	// before we start any other workers that need to connect
	// to the controller over the controller port.
	w.updateControllerPort(config.ControllerAPIPort)

	unsub, err := config.Hub.Subscribe(controller.ConfigChanged,
		func(topic string, data controller.ConfigChangedMessage, err error) {
			if w.updateControllerPort(data.Config.ControllerAPIPort()) {
				w.tomb.Kill(dependency.ErrBounce)
			}
		})
	if err != nil {
		return nil, errors.Annotate(err, "unable to subscribe to details topic")
	}

	w.tomb.Go(func() error {
		defer unsub()
		<-w.tomb.Dying()
		return nil
	})
	return w, nil
}

// Worker is responsible for updating the agent config when the controller api
// port value changes, and then bouncing the worker to notify the other
// dependent workers, which should be the peer-grouper and http-server.
type Worker struct {
	tomb   tomb.Tomb
	logger Logger
	update func(int) error

	controllerAPIPort int
}

// Kill implements Worker.Kill.
func (w *Worker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements Worker.Wait.
func (w *Worker) Wait() error {
	return w.tomb.Wait()
}

func (w *Worker) updateControllerPort(port int) bool {
	if w.controllerAPIPort == port {
		return false
	}
	// The local cache is out of date, update it.
	w.logger.Infof("updating controller API port to %v", port)
	err := w.update(port)
	if err != nil {
		w.logger.Errorf("unable to update agent.conf with new controller API port: %v", err)
	}
	w.controllerAPIPort = port
	return true
}
