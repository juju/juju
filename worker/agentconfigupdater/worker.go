// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentconfigupdater

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/mongo"
	controllermsg "github.com/juju/juju/pubsub/controller"
)

// Controller defines the methods that the worker requires
// to get the controller config and state serving info.
type Controller interface {
	ControllerConfig() (controller.Config, error)
	StateServingInfo() (params.StateServingInfo, error)
}

// WorkerConfig contains the information necessary to run
// the agent config updater worker.
type WorkerConfig struct {
	Agent      coreagent.Agent
	Controller Controller
	Hub        *pubsub.StructuredHub
	Logger     Logger
}

// Validate ensures that the required values are set in the structure.
func (c *WorkerConfig) Validate() error {
	if c.Agent == nil {
		return errors.NotValidf("missing agent")
	}
	if c.Controller == nil {
		return errors.NotValidf("missing controller")
	}
	if c.Hub == nil {
		return errors.NotValidf("missing hub")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

type agentConfigUpdater struct {
	config WorkerConfig

	tomb         tomb.Tomb
	mongoProfile mongo.MemoryProfile
}

// NewWorker creates a new agent config updater worker.
func NewWorker(config WorkerConfig) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfig, err := config.Controller.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "getting controller config")
	}
	// If the mongo memory profile from the controller config
	// is different from the one in the agent config we need to
	// restart the agent to apply the memory profile to the mongo
	// service.
	agentsMongoMemoryProfile := config.Agent.CurrentConfig().MongoMemoryProfile()
	configMongoMemoryProfile := mongo.MemoryProfile(controllerConfig.MongoMemoryProfile())
	mongoProfileChanged := agentsMongoMemoryProfile != configMongoMemoryProfile

	info, err := config.Controller.StateServingInfo()
	if err != nil {
		return nil, errors.Annotate(err, "getting state serving info")
	}
	err = agent.ChangeConfig(func(config coreagent.ConfigSetter) error {
		existing, hasInfo := config.StateServingInfo()
		if hasInfo {
			// Use the existing cert and key as they appear to
			// have been already updated by the cert updater
			// worker to have this machine's IP address as
			// part of the cert. This changed cert is never
			// put back into the database, so it isn't
			// reflected in the copy we have got from
			// apiState.
			info.Cert = existing.Cert
			info.PrivateKey = existing.PrivateKey
		}
		config.SetStateServingInfo(info)
		if mongoProfileChanged {
			config.Logger.Debugf("setting agent config mongo memory profile: %s", mongoProfile)
			config.SetMongoMemoryProfile(configMongoMemoryProfile)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	// If we need a restart, return the fatal error.
	if mongoProfileChanged {
		config.Logger.Infof("restarting agent for new mongo memory profile")
		return nil, jworker.ErrRestartAgent
	}

	started := make(chan struct{})
	w := &agentConfigUpdater{
		config:       config,
		mongoProfile: configMongoMemoryProfile,
	}
	w.tomb.Go(func() error {
		return w.loop(started)
	})
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		return nil, errors.New("worker failed to start properly")
	}
	return w, nil
}

func (w *agentConfigUpdater) loop(started chan struct{}) error {
	unsubscribe, err = w.config.Hub.Subscribe(controllermsg.ConfigChanged, w.onConfigChanged)
	if err != nil {
		ctx.logger.Criticalf("programming error in subscribe function: %v", err)
		return errors.Trace(err)
	}
	defer unsubscribe()
	// Let the caller know we are done.
	close(started)
	// Don't exit until we are told to. Exiting unsubscribes.
	<-w.tomb.Dying()
	w.config.Logger.Tracef("agentConfigUpdater loop finished")
	return nil
}

func (w *agentConfigUpdater) onConfigChanged(topic string, data controllermsg.ConfigChangedMessage, err error) {
	if err != nil {
		w.config.Logger.Criticalf("programming error in %s message data: %v", topic, err)
		return
	}

	mongoProfile := mongo.MemoryProfile(data.Config.MongoMemoryProfile())
	if mongoProfile == w.mongoProfile {
		// Nothing to do, all good.
		return
	}

	err = agent.ChangeConfig(func(config coreagent.ConfigSetter) error {
		w.config.Logger.Debugf("setting agent config mongo memory profile: %s", mongoProfile)
		config.SetMongoMemoryProfile(mongoProfile)
		return nil
	})
	if err != nil {
		w.config.Logger.Warningf("failed to update agent config: %v", err)
		return
	}

	w.tomb.Kill(jworker.ErrRestartAgent)
}
