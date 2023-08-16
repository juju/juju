// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllercharm

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/pubsub/agent"
)

// Facade exposes controller functionality to a Worker.
type Facade interface {
	AddMetricsUser(username, password string) error
	RemoveMetricsUser(username string) error
}

// Logger allows logging messages.
type Logger interface {
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Errorf(string, ...interface{})
}

// Config holds configuration for the controllercharm worker.
type Config struct {
	Facade Facade
	Hub    Hub
	Logger Logger
}

// Validate validates the worker configuration.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("missing Facade")
	}
	if config.Hub == nil {
		return errors.NotValidf("missing Hub")
	}
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

type controllerCharmWorker struct {
	Config
	catacomb catacomb.Catacomb
	unsub    func()
}

// NewWorker starts and returns a new controllercharm worker.
func NewWorker(config Config) (worker.Worker, error) {
	w := controllerCharmWorker{
		Config: config,
	}

	unsubAdd := w.Hub.Subscribe(agent.AddMetricsUserTopic, w.handleAddMetricsUserRequest)
	unsubRemove := w.Hub.Subscribe(agent.RemoveMetricsUserTopic, w.handleRemoveMetricsUserRequest)
	w.unsub = func() {
		unsubAdd()
		unsubRemove()
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	w.Logger.Infof("controllercharm worker started")
	return &w, nil
}

func (w *controllerCharmWorker) handleAddMetricsUserRequest(_ string, data interface{}) {
	w.Logger.Debugf("received add-metrics-user request")
	userInfo, ok := data.(agent.UserInfo)
	if !ok {
		w.Logger.Errorf("data for add-metrics-user request should be a UserInfo structure")
		return
	}
	w.Logger.Debugf("add-metrics-user request for username %q", userInfo.Username)

	err := w.Facade.AddMetricsUser(userInfo.Username, userInfo.Password)
	w.Logger.Debugf("response to add-metrics-user request: %s", err)

	var response string
	if err == nil {
		response = fmt.Sprintf("successfully created user %s", userInfo.Username)
	} else {
		response = fmt.Sprintf("error creating user %s: %s", userInfo.Username, err.Error())
	}
	w.Hub.Publish(agent.AddMetricsUserResponseTopic, response)
}

func (w *controllerCharmWorker) handleRemoveMetricsUserRequest(_ string, data interface{}) {
	w.Logger.Debugf("received remove-metrics-user request: %#v", data)
	username, ok := data.(string)
	if !ok {
		w.Logger.Errorf("data for remove-metrics-user request should be a string representing username")
		return
	}

	err := w.Facade.RemoveMetricsUser(username)
	w.Logger.Debugf("response to remove-metrics-user request: %s", err)

	var response string
	if err == nil {
		response = fmt.Sprintf("successfully removed user %s", username)
	} else {
		response = fmt.Sprintf("error removing user %s: %s", username, err.Error())
	}
	w.Hub.Publish(agent.RemoveMetricsUserResponseTopic, response)
}

// Kill is part of the worker.Worker interface.
func (w *controllerCharmWorker) Kill() {
	w.unsub()
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *controllerCharmWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *controllerCharmWorker) loop() error {
	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}
