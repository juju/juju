// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevision

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	charmmetrics "github.com/juju/juju/core/charm/metrics"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
	internalerrors "github.com/juju/juju/internal/errors"
)

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)

	// Watch returns a watcher that notifies of changes to the model config.
	Watch() (watcher.StringsWatcher, error)
}

// ApplicationService provides access to applications.
type ApplicationService interface {
}

// ModelService provides access to the model.
type ModelService interface {
	// GetModelMetrics returns the model metrics information set in the
	// database.
	GetModelMetrics(context.Context) (coremodel.ModelMetrics, error)
}

// Config defines the operation of a charm revision updater worker.
type Config struct {
	// ModelConfigService is the service used to access model configuration.
	ModelConfigService ModelConfigService

	// ApplicationService is the service used to access applications.
	ApplicationService ApplicationService

	// ModelService is the service used to access the model.
	ModelService ModelService

	// Clock is the worker's view of time.
	Clock clock.Clock

	// Period is the time between charm revision updates.
	Period time.Duration

	// Logger is the logger used for debug logging in this worker.
	Logger logger.Logger
}

// Validate returns an error if the configuration cannot be expected
// to start a functional worker.
func (config Config) Validate() error {
	if config.ModelConfigService == nil {
		return errors.NotValidf("nil ModelConfigService")
	}
	if config.ApplicationService == nil {
		return errors.NotValidf("nil ApplicationService")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Period <= 0 {
		return errors.NotValidf("non-positive Period")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

type revisionUpdateWorker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// NewWorker returns a worker that calls UpdateLatestRevisions on the
// configured RevisionUpdater, once when started and subsequently every
// Period.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, internalerrors.Capture(err)
	}
	w := &revisionUpdateWorker{
		config: config,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, internalerrors.Capture(err)
	}

	w.config.Logger.Debugf("worker created with period %v", w.config.Period)

	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *revisionUpdateWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *revisionUpdateWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *revisionUpdateWorker) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	// Watch the model config for new charmhub URL values, so we can swap the
	// charmhub client to use the new URL.

	modelConfigService := w.config.ModelConfigService
	configWatcher, err := modelConfigService.Watch()
	if err != nil {
		return internalerrors.Capture(err)
	}

	if err := w.catacomb.Add(configWatcher); err != nil {
		return internalerrors.Capture(err)
	}

	logger := w.config.Logger
	logger.Debugf("watching model config for changes to charmhub URL")

	charmhubClient, err := w.getCharmhubClient(ctx)
	if err != nil {
		return internalerrors.Capture(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-w.config.Clock.After(jitter(w.config.Period)):
			w.config.Logger.Debugf("%v elapsed, performing work", w.config.Period)

			if err := w.update(ctx, charmhubClient); err != nil {
				return internalerrors.Capture(err)
			}

		case change, ok := <-configWatcher.Changes():
			if !ok {
				return errors.New("model config watcher closed")
			}

			var refresh bool
			for _, key := range change {
				if key == config.CharmHubURLKey {
					refresh = true
					break
				}
			}

			if !refresh {
				continue
			}

			logger.Debugf("refreshing charmhubClient due to model config change")

			charmhubClient, err = w.getCharmhubClient(ctx)
			if err != nil {
				return internalerrors.Capture(err)
			}
		}
	}
}

func (w *revisionUpdateWorker) update(ctx context.Context, client CharmhubClient) error {
	service := w.config.ApplicationService
	applications, err := service.GetAllCharmhubApplications(ctx)
	if err != nil {
		return internalerrors.Capture(err)
	}

	cfg, err := w.config.ModelConfigService.ModelConfig(ctx)
	if err != nil {
		return internalerrors.Capture(err)
	}

	telemetry := cfg.Telemetry()

	if len(applications) == 0 {
		if !telemetry {
			return nil
		}
		return w.sendEmptyModelMetrics(ctx, client)
	}

}

func (w *revisionUpdateWorker) sendEmptyModelMetrics(ctx context.Context, client CharmhubClient) error {

	return nil
}

func (w *revisionUpdateWorker) getCharmhubClient(ctx context.Context) (Downloader, error) {
	// Get a new downloader, this ensures that we've got a fresh
	// connection to the charm store.
	httpClient, err := w.config.NewHTTPClient(ctx, w.config.HTTPClientGetter)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	config, err := w.config.ModelConfigService.ModelConfig(ctx)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	charmhubURL, _ := config.CharmHubURL()

	return w.config.NewCharmhubClient(httpClient, charmhubURL, w.config.Logger)
}

func (w *revisionUpdateWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

// charmhubRequestMetadata returns a map containing metadata key/value pairs to
// send to the charmhub for tracking metrics.
func (w *revisionUpdateWorker) charmhubRequestMetadata(ctx context.Context) (map[charmmetrics.MetricKey]map[charmmetrics.MetricKey]string, error) {
	metrics, err := w.config.ModelService.GetModelMetrics(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	metadata := map[charmmetrics.MetricKey]map[charmmetrics.MetricKey]string{
		charmmetrics.Controller: {
			charmmetrics.JujuVersion: version.Current.String(),
			charmmetrics.UUID:        metrics.ControllerUUID.String(),
		},
		charmmetrics.Model: {
			charmmetrics.UUID:     metrics.UUID.String(),
			charmmetrics.Cloud:    metrics.Cloud,
			charmmetrics.Provider: metrics.CloudType,
			charmmetrics.Region:   metrics.CloudRegion,

			charmmetrics.NumApplications: metrics.ApplicationCount,
			charmmetrics.NumMachines:     metrics.MachineCount,
			charmmetrics.NumUnits:        metrics.UnitCount,
		},
	}

	return metadata, nil
}

func jitter(period time.Duration) time.Duration {
	return retry.ExpBackoff(period, period*2, 2, true)(0, 1)
}
