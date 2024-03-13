// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// ConfigObserver exposes a model configuration and a watch constructor
// that allows clients to be informed of changes to the configuration.
type ConfigObserver interface {
	environs.EnvironConfigGetter
	WatchForModelConfigChanges() (watcher.NotifyWatcher, error)
	WatchCloudSpecChanges() (watcher.NotifyWatcher, error)
}

// Config describes the dependencies of a Worker.
//
// It's arguable that it should be called WorkerConfig, because of the heavy
// use of model config in this package.
type Config struct {
	Observer       ConfigObserver
	NewEnvironFunc environs.NewEnvironFunc
	Logger         Logger
}

// Validate returns an error if the config cannot be used to start a Worker.
func (config Config) Validate() error {
	if config.Observer == nil {
		return errors.NotValidf("nil Observer")
	}
	if config.NewEnvironFunc == nil {
		return errors.NotValidf("nil NewEnvironFunc")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Worker loads an environment, makes it available to clients, and updates
// the environment in response to config changes until it is killed.
type Worker struct {
	config           Config
	catacomb         catacomb.Catacomb
	environ          environs.Environ
	currentCloudSpec environscloudspec.CloudSpec
}

// NewWorker loads a provider from the observer and returns a new Worker,
// or an error if anything goes wrong. If a tracker is returned, its Environ()
// method is immediately usable.
func NewWorker(ctx context.Context, config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	environ, spec, err := environs.GetEnvironAndCloud(ctx, config.Observer, config.NewEnvironFunc)
	if err != nil {
		return nil, errors.Trace(err)
	}

	t := &Worker{
		config:           config,
		environ:          environ,
		currentCloudSpec: *spec,
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &t.catacomb,
		Work: t.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return t, nil
}

// Environ returns the encapsulated Environ. It will continue to be updated in
// the background for as long as the Worker continues to run.
func (t *Worker) Environ() environs.Environ {
	return t.environ
}

func (t *Worker) loop() (err error) {
	cfg := t.environ.Config()
	defer errors.DeferredAnnotatef(&err, "model %q (%s)", cfg.Name(), cfg.UUID())

	logger := t.config.Logger
	environWatcher, err := t.config.Observer.WatchForModelConfigChanges()
	if err != nil {
		return errors.Annotate(err, "watching environ config")
	}
	if err := t.catacomb.Add(environWatcher); err != nil {
		return errors.Trace(err)
	}

	// Some environs support reacting to changes in the cloud config.
	// Set up a watcher if that's the case.
	var (
		cloudWatcherChanges watcher.NotifyChannel
		cloudSpecSetter     environs.CloudSpecSetter
		ok                  bool
	)
	if cloudSpecSetter, ok = t.environ.(environs.CloudSpecSetter); !ok {
		logger.Warningf("cloud type %v doesn't support dynamic changing of cloud spec", t.environ.Config().Type())
	} else {
		cloudWatcher, err := t.config.Observer.WatchCloudSpecChanges()
		if err != nil {
			return errors.Annotate(err, "cannot watch environ cloud spec")
		}
		if err := t.catacomb.Add(cloudWatcher); err != nil {
			return errors.Trace(err)
		}
		cloudWatcherChanges = cloudWatcher.Changes()
	}
	for {
		logger.Debugf("waiting for environ watch notification")
		select {
		case <-t.catacomb.Dying():
			return t.catacomb.ErrDying()
		case _, ok := <-environWatcher.Changes():
			if !ok {
				return errors.New("environ config watch closed")
			}
			logger.Debugf("reloading environ config")
			modelConfig, err := t.config.Observer.ModelConfig(context.TODO())
			if err != nil {
				return errors.Annotate(err, "reading model config")
			}
			if err = t.environ.SetConfig(modelConfig); err != nil {
				return errors.Annotate(err, "updating environ config")
			}
		case _, ok := <-cloudWatcherChanges:
			if !ok {
				return errors.New("cloud watch closed")
			}
			cloudSpec, err := t.config.Observer.CloudSpec(context.TODO())
			if err != nil {
				return errors.Annotate(err, "reading environ cloud spec")
			}
			if reflect.DeepEqual(cloudSpec, t.currentCloudSpec) {
				continue
			}
			logger.Debugf("reloading cloud config")
			if err = cloudSpecSetter.SetCloudSpec(context.TODO(), cloudSpec); err != nil {
				return errors.Annotate(err, "cannot update environ cloud spec")
			}
			t.currentCloudSpec = cloudSpec
		}
	}
}

// Kill is part of the worker.Worker interface.
func (t *Worker) Kill() {
	t.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (t *Worker) Wait() error {
	return t.catacomb.Wait()
}
