// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasbroker

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

// ConfigAPI exposes a model configuration and a watch constructor
// that allows clients to be informed of changes to the configuration.
type ConfigAPI interface {
	CloudSpec(context.Context) (environscloudspec.CloudSpec, error)
	ModelConfig(context.Context) (*config.Config, error)
	ControllerConfig(context.Context) (controller.Config, error)
	WatchForModelConfigChanges(context.Context) (watcher.NotifyWatcher, error)
	WatchCloudSpecChanges(context.Context) (watcher.NotifyWatcher, error)
}

// Config describes the dependencies of a Tracker.
//
// It's arguable that it should be called TrackerConfig, because of the heavy
// use of model config in this package.
type Config struct {
	ConfigAPI              ConfigAPI
	NewContainerBrokerFunc caas.NewContainerBrokerFunc
	Logger                 logger.Logger
}

// Validate returns an error if the config cannot be used to start a Tracker.
func (config Config) Validate() error {
	if config.ConfigAPI == nil {
		return errors.NotValidf("nil ConfigAPI")
	}
	if config.NewContainerBrokerFunc == nil {
		return errors.NotValidf("nil NewContainerBrokerFunc")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Tracker loads a caas broker, makes it available to clients, and updates
// the broker in response to config changes until it is killed.
type Tracker struct {
	config           Config
	catacomb         catacomb.Catacomb
	broker           caas.Broker
	currentCloudSpec environscloudspec.CloudSpec
}

// NewTracker returns a new Tracker, or an error if anything goes wrong.
// If a tracker is returned, its Broker() method is immediately usable.
//
// The caller is responsible for Kill()ing the returned Tracker and Wait()ing
// for any errors it might return.
func NewTracker(ctx context.Context, config Config) (*Tracker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpec, err := config.ConfigAPI.CloudSpec(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get cloud information")
	}
	cfg, err := config.ConfigAPI.ModelConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ctrlCfg, err := config.ConfigAPI.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	broker, err := config.NewContainerBrokerFunc(ctx, environs.OpenParams{
		ControllerUUID: ctrlCfg.ControllerUUID(),
		Cloud:          cloudSpec,
		Config:         cfg,
	}, environs.NoopCredentialInvalidator())
	if err != nil {
		return nil, errors.Annotate(err, "cannot create caas broker")
	}

	t := &Tracker{
		config:           config,
		broker:           broker,
		currentCloudSpec: cloudSpec,
	}
	err = catacomb.Invoke(catacomb.Plan{
		Name: "caas-broker-tracker",
		Site: &t.catacomb,
		Work: t.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return t, nil
}

// Broker returns the encapsulated Broker. It will continue to be updated in
// the background for as long as the Tracker continues to run.
func (t *Tracker) Broker() caas.Broker {
	return t.broker
}

func (t *Tracker) loop() error {
	logger := t.config.Logger

	ctx, cancel := t.scopedContext()
	defer cancel()

	modelWatcher, err := t.config.ConfigAPI.WatchForModelConfigChanges(ctx)
	if err != nil {
		return errors.Annotate(err, "cannot watch model config")
	}
	if err := t.catacomb.Add(modelWatcher); err != nil {
		return errors.Trace(err)
	}

	// Some environs support reacting to changes in the cloud config.
	// Set up a watcher if that's the case.
	var (
		cloudWatcherChanges watcher.NotifyChannel
		cloudSpecSetter     environs.CloudSpecSetter
		ok                  bool
	)
	if cloudSpecSetter, ok = t.broker.(environs.CloudSpecSetter); !ok {
		logger.Warningf(ctx, "cloud type %v doesn't support dynamic changing of cloud spec", t.broker.Config().Type())
	} else {
		cloudWatcher, err := t.config.ConfigAPI.WatchCloudSpecChanges(ctx)
		if err != nil {
			return errors.Annotate(err, "cannot watch environ cloud spec")
		}
		if err := t.catacomb.Add(cloudWatcher); err != nil {
			return errors.Trace(err)
		}
		cloudWatcherChanges = cloudWatcher.Changes()
	}

	for {
		logger.Debugf(ctx, "waiting for config and credential notifications")
		select {
		case <-t.catacomb.Dying():
			return t.catacomb.ErrDying()
		case _, ok := <-modelWatcher.Changes():
			if !ok {
				return errors.New("model config watch closed")
			}
			logger.Debugf(ctx, "reloading model config")
			modelConfig, err := t.config.ConfigAPI.ModelConfig(context.TODO())
			if err != nil {
				return errors.Annotate(err, "cannot read model config")
			}
			if err = t.broker.SetConfig(ctx, modelConfig); err != nil {
				return errors.Annotate(err, "cannot update model config")
			}
		case _, ok := <-cloudWatcherChanges:
			if !ok {
				return errors.New("cloud watch closed")
			}
			cloudSpec, err := t.config.ConfigAPI.CloudSpec(context.TODO())
			if err != nil {
				return errors.Annotate(err, "cannot read model config")
			}
			if reflect.DeepEqual(cloudSpec, t.currentCloudSpec) {
				continue
			}
			logger.Debugf(ctx, "reloading cloud config")
			if err = cloudSpecSetter.SetCloudSpec(ctx, cloudSpec); err != nil {
				return errors.Annotate(err, "cannot update broker cloud spec")
			}
			t.currentCloudSpec = cloudSpec
		}
	}
}

// Kill is part of the worker.Worker interface.
func (t *Tracker) Kill() {
	t.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (t *Tracker) Wait() error {
	return t.catacomb.Wait()
}

func (t *Tracker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(t.catacomb.Context(context.Background()))
}
