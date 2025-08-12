// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisioner

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	charmmetrics "github.com/juju/juju/core/charm/metrics"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	domainresource "github.com/juju/juju/domain/resource"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/repository"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	internalerrors "github.com/juju/juju/internal/errors"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
)

const (
	// ErrFailedToSendMetrics is the error returned when sending metrics to the
	// charmhub fails.
	ErrFailedToSendMetrics = internalerrors.ConstError("sending metrics failed")
)

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)

	// Watch returns a watcher that notifies of changes to the model config.
	Watch(context.Context) (watcher.StringsWatcher, error)
}

// ApplicationService provides access to applications.
type ApplicationService interface {

	// GetApplicationIDByName returns an application ID by application name. It
	// returns an error if the application can not be found by the name.
	//
	// Returns [applicationerrors.ApplicationNotFound] if the application is not found.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// GetApplicationsForRevisionUpdater returns the applications that should be
	// used by the revision updater.
	GetApplicationsForRevisionUpdater(context.Context) ([]application.RevisionUpdaterApplication, error)

	// ReserveCharmRevision reserves a charm revision for the given charm id.
	// If there are any non-blocking issues with the charm metadata, actions,
	// config or manifest, a set of warnings will be returned.
	ReserveCharmRevision(ctx context.Context, args applicationcharm.ReserveCharmRevisionArgs) (corecharm.ID, []string, error)
}

// ResourceService defines the interface for managing resources associated with
// applications.
type ResourceService interface {
	// SetRepositoryResources updates the last available revision of resources
	// from charm repository for a specific application.
	SetRepositoryResources(ctx context.Context, args domainresource.SetRepositoryResourcesArgs) error
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

	// ResourceService is the service for managing resources
	ResourceService ResourceService

	// ModelTag is the tag of the model the worker is running in.
	ModelTag names.ModelTag

	// HTTPClientGetter is the getter used to create HTTP clients.
	HTTPClientGetter corehttp.HTTPClientGetter

	// NewHTTPClient is the function used to create a new HTTP client.
	NewHTTPClient NewHTTPClientFunc

	// NewCharmhubClient is the function used to create a new CharmhubClient.
	NewCharmhubClient NewCharmhubClientFunc

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
	if config.ModelService == nil {
		return errors.NotValidf("nil ModelService")
	}
	if config.ResourceService == nil {
		return errors.NotValidf("nil ResourceService")
	}
	if config.HTTPClientGetter == nil {
		return errors.NotValidf("nil HTTPClientGetter")
	}
	if config.NewHTTPClient == nil {
		return errors.NotValidf("nil NewHTTPClient")
	}
	if config.NewCharmhubClient == nil {
		return errors.NotValidf("nil NewCharmhubClient")
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
	internalStates chan string
	catacomb       catacomb.Catacomb
	config         Config
}

// NewWorker returns a worker that calls UpdateLatestRevisions on the
// configured RevisionUpdater, once when started and subsequently every
// Period.
func NewWorker(config Config) (worker.Worker, error) {
	return newWorker(config, nil)
}

// NewWorker returns a worker that calls UpdateLatestRevisions on the
// configured RevisionUpdater, once when started and subsequently every
// Period.
func newWorker(config Config, internalState chan string) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, internalerrors.Capture(err)
	}
	w := &revisionUpdateWorker{
		internalStates: internalState,
		config:         config,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "charm-revision-updater",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, internalerrors.Capture(err)
	}

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

	w.config.Logger.Debugf(ctx, "worker created with period %v", w.config.Period)

	// Watch the model config for new charmhub URL values, so we can swap the
	// charmhub client to use the new URL.

	modelConfigService := w.config.ModelConfigService
	configWatcher, err := modelConfigService.Watch(ctx)
	if err != nil {
		return internalerrors.Capture(err)
	}

	if err := w.catacomb.Add(configWatcher); err != nil {
		return internalerrors.Capture(err)
	}

	logger := w.config.Logger
	logger.Debugf(ctx, "watching model config for changes to charmhub URL")

	charmhubClient, err := w.getCharmhubClient(ctx)
	if err != nil {
		return internalerrors.Capture(err)
	}

	// Report the initial started state.
	w.reportInternalState(stateStarted)

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-w.config.Clock.After(jitter(w.config.Period)):
			w.config.Logger.Debugf(ctx, "%v elapsed, performing work", w.config.Period)

			// This worker is responsible for updating the latest revision of
			// applications in the model. It does this by fetching the latest
			// revision from the charmhub and updating the model with the
			// information.
			// If the update fails, the worker will log an error and continue
			// to the next application.

			latestInfo, err := w.fetch(ctx, charmhubClient)
			if errors.Is(err, ErrFailedToSendMetrics) {
				logger.Warningf(ctx, "failed to send metrics: %v", err)
				continue
			} else if err != nil {
				logger.Errorf(ctx, "failed to fetch revisions: %v", err)
				continue
			} else if len(latestInfo) == 0 {
				if err := w.recordNoApplications(ctx, charmhubClient); err != nil {
					logger.Warningf(ctx, "failed to record no applications: %v", err)
				}
				logger.Debugf(ctx, "no new application revisions")
				continue
			}

			logger.Debugf(ctx, "revisions fetched for %d applications", len(latestInfo))

			if err := w.storeNewRevisions(ctx, latestInfo); err != nil {
				logger.Warningf(ctx, "failed to store revisions: %v", err)
				continue
			}

			logger.Debugf(ctx, "revisions stored for %d applications", len(latestInfo))

		case changes, ok := <-configWatcher.Changes():
			if !ok {
				return errors.New("model config watcher closed")
			}

			var refresh bool
			for _, key := range changes {
				if key == config.CharmHubURLKey {
					refresh = true
					break
				}
			}

			if !refresh {
				continue
			}

			logger.Debugf(ctx, "refreshing charmhubClient due to model config change")

			charmhubClient, err = w.getCharmhubClient(ctx)
			if err != nil {
				return internalerrors.Capture(err)
			}
		}
	}
}

func (w *revisionUpdateWorker) fetch(ctx context.Context, client CharmhubClient) ([]latestCharmInfo, error) {
	service := w.config.ApplicationService
	applications, err := service.GetApplicationsForRevisionUpdater(ctx)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	if len(applications) == 0 {
		return nil, nil
	}

	buildTelemetry, err := w.shouldBuildTelemetry(ctx)
	if err != nil {
		w.config.Logger.Infof(ctx, "checking telemetry config: %v", err)
		buildTelemetry = false
	}

	charmhubIDs := make([]charmhubID, len(applications))
	charmhubApps := make([]appInfo, len(applications))

	for i, app := range applications {
		charmhubID, err := encodeCharmhubID(app, w.config.ModelTag)
		if err != nil {
			w.config.Logger.Infof(ctx, "encoding charmhub ID for %q: %v", app.Name, err)
			continue
		}

		if buildTelemetry {
			charmhubID.metrics = map[charmmetrics.MetricValueKey]string{
				charmmetrics.NumUnits: strconv.Itoa(app.NumUnits),
			}
		}

		charmhubIDs[i] = charmhubID
		charmhubApps[i] = appInfo{
			name:         app.Name,
			charmLocator: app.CharmLocator,
			origin:       app.Origin,
		}
	}

	return w.fetchInfo(ctx, client, buildTelemetry, charmhubIDs, charmhubApps)
}

func (w *revisionUpdateWorker) recordNoApplications(ctx context.Context, client CharmhubClient) error {
	buildTelemetry, err := w.shouldBuildTelemetry(ctx)
	if err != nil {
		w.config.Logger.Infof(ctx, "checking telemetry config: %v", err)
		buildTelemetry = false
	}

	return w.sendEmptyModelMetrics(ctx, client, buildTelemetry)
}

func (w *revisionUpdateWorker) shouldBuildTelemetry(ctx context.Context) (bool, error) {
	cfg, err := w.config.ModelConfigService.ModelConfig(ctx)
	if err != nil {
		return false, internalerrors.Capture(err)
	}

	return cfg.Telemetry(), nil
}

func (w *revisionUpdateWorker) sendEmptyModelMetrics(ctx context.Context, client CharmhubClient, buildTelemetry bool) error {
	metadata, err := w.buildMetricsMetadata(ctx, buildTelemetry)
	if err != nil {
		return internalerrors.Capture(err)
	} else if len(metadata) == 0 {
		return nil
	}

	// Override the context which will use a shorter timeout for sending
	// metrics.
	ctx, cancel := context.WithTimeout(ctx, charmhub.RefreshTimeout)
	defer cancel()

	if err := client.RefreshWithMetricsOnly(ctx, metadata); err != nil {
		return internalerrors.Errorf("%w: %w", ErrFailedToSendMetrics, err)
	}

	return nil
}

func (w *revisionUpdateWorker) fetchInfo(ctx context.Context, client CharmhubClient, buildTelemetry bool, ids []charmhubID, apps []appInfo) ([]latestCharmInfo, error) {
	metrics, err := w.buildMetricsMetadata(ctx, buildTelemetry)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	// Override the context which will use a shorter timeout for sending
	// metrics.
	ctx, cancel := context.WithTimeout(ctx, charmhub.RefreshTimeout)
	defer cancel()

	response, err := w.request(ctx, client, metrics, ids)
	if err != nil {
		return nil, internalerrors.Errorf("requesting latest information: %w", err)
	}

	if len(response) != len(apps) {
		return nil, internalerrors.Errorf("expected %d responses, got %d", len(apps), len(response))
	}

	var latest []latestCharmInfo
	for i, result := range response {
		// The application platform architecture can't change during the
		// lifecycle of an application. We therefore must use the platform from
		// the origin of the application.
		origin := apps[i].origin
		arch, err := encodeArchitecture(origin.Platform.Architecture)
		if err != nil {
			return nil, internalerrors.Errorf("encoding architecture: %w", err)
		}

		osType, err := encodeOSType(origin.Platform.OSType)
		if err != nil {
			return nil, internalerrors.Errorf("encoding os type: %w", err)
		}

		essentialMetadata := result.essentialMetadata
		essentialMetadata.ResolvedOrigin.Platform = corecharm.Platform{
			Architecture: arch,
			OS:           osType,
			Channel:      origin.Platform.Channel,
		}

		latest = append(latest, latestCharmInfo{
			charmLocator:      apps[i].charmLocator,
			essentialMetadata: essentialMetadata,
			timestamp:         result.timestamp,
			revision:          result.revision,
			resources:         result.resources,
			appName:           apps[i].name,
		})
	}

	return latest, nil
}

// request fetches the latest information about the given charms from charmhub's
// "charm_refresh" API.
func (w *revisionUpdateWorker) request(ctx context.Context, client CharmhubClient, metrics charmhub.Metrics, ids []charmhubID) ([]charmhubResult, error) {
	configs := make([]charmhub.RefreshConfig, len(ids))
	for i, id := range ids {
		base := charmhub.RefreshBase{
			Architecture: id.arch,
			Name:         id.osType,
			Channel:      id.osChannel,
		}
		cfg, err := charmhub.RefreshOne(ctx, id.instanceKey, id.id, id.revision, id.channel, base)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		cfg, err = charmhub.AddConfigMetrics(cfg, id.metrics)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		configs[i] = cfg
	}
	config := charmhub.RefreshMany(configs...)

	ctx, cancel := context.WithTimeout(ctx, charmhub.RefreshTimeout)
	defer cancel()

	w.config.Logger.Debugf(ctx, "refreshing %d charms", len(configs))

	responses, err := client.RefreshWithRequestMetrics(ctx, config, metrics)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	if len(responses) != len(ids) {
		return nil, internalerrors.Errorf("expected %d responses, got %d", len(ids), len(responses))
	}

	results := make([]charmhubResult, len(responses))
	for i, response := range responses {
		result, err := w.refreshResponseToCharmhubResult(ctx, response)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		results[i] = result
	}
	return results, nil
}

func (w *revisionUpdateWorker) storeNewRevisions(ctx context.Context, latestInfo []latestCharmInfo) error {
	for _, info := range latestInfo {
		if err := w.storeNewCharmRevision(ctx, info); err != nil {
			return internalerrors.Capture(err)
		}
	}
	return nil
}

func (w *revisionUpdateWorker) storeNewCharmRevision(ctx context.Context, info latestCharmInfo) error {
	// Insert the new charm revision into the model.
	service := w.config.ApplicationService

	essentialMetadata := info.essentialMetadata
	downloadInfo := essentialMetadata.DownloadInfo
	origin := essentialMetadata.ResolvedOrigin

	charmID, warnings, err := service.ReserveCharmRevision(ctx, applicationcharm.ReserveCharmRevisionArgs{
		Charm: charm.NewCharmBase(
			essentialMetadata.Meta,
			essentialMetadata.Manifest,
			essentialMetadata.Config,
			// These will be filled in once we have all the data in the
			// response from the charmhub.
			nil, nil,
		),
		// This will always be a charmhub charm.
		Source:        corecharm.CharmHub,
		ReferenceName: info.charmLocator.Name,
		// This is the new revision located from the fetch.
		Revision: info.revision,
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: downloadInfo.CharmhubIdentifier,
			DownloadURL:        downloadInfo.DownloadURL,
			DownloadSize:       downloadInfo.DownloadSize,
		},
		Hash:         origin.Hash,
		Architecture: origin.Platform.Architecture,
	})
	if err != nil {
		return internalerrors.Capture(err)
	} else if len(warnings) > 0 {
		w.config.Logger.Infof(ctx, "reserving charm revision for %q: %v", info.appName, warnings)
	}

	return w.storeNewResourcesRevision(ctx, charmID, info)
}

func (w *revisionUpdateWorker) storeNewResourcesRevision(ctx context.Context,
	charmID corecharm.ID, info latestCharmInfo) error {
	// Skip updates resources revision if there is none.
	if len(info.resources) == 0 {
		return nil
	}

	// Store resources revision.
	appID, err := w.config.ApplicationService.GetApplicationIDByName(ctx, info.appName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		// Maybe the application has been removed in the meantime. In this case,
		// that's not a real issue. Log it as a warning and continue.
		w.config.Logger.Warningf(ctx, "failed to get application ID for %q: %v", info.appName, err)
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return w.config.ResourceService.SetRepositoryResources(ctx, domainresource.SetRepositoryResourcesArgs{
		ApplicationID: appID,
		CharmID:       charmID,
		Info:          info.resources,
		LastPolled:    w.config.Clock.Now(),
	})
}

func (w *revisionUpdateWorker) getCharmhubClient(ctx context.Context) (CharmhubClient, error) {
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

// buildMetricsMetadata returns a map containing metadata key/value pairs to
// send to the charmhub for tracking metrics.
func (w *revisionUpdateWorker) buildMetricsMetadata(ctx context.Context, buildTelemetry bool) (charmhub.Metrics, error) {
	if !buildTelemetry {
		return nil, nil
	}

	metrics, err := w.config.ModelService.GetModelMetrics(ctx)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	model := metrics.Model

	return charmhub.Metrics{
		charmmetrics.Controller: {
			charmmetrics.JujuVersion: version.Current.String(),
			charmmetrics.UUID:        model.ControllerUUID.String(),
		},
		charmmetrics.Model: {
			charmmetrics.UUID:     model.UUID.String(),
			charmmetrics.Cloud:    model.Cloud,
			charmmetrics.Provider: model.CloudType,
			charmmetrics.Region:   model.CloudRegion,

			charmmetrics.NumApplications: strconv.Itoa(metrics.ApplicationCount),
			charmmetrics.NumMachines:     strconv.Itoa(metrics.MachineCount),
			charmmetrics.NumUnits:        strconv.Itoa(metrics.UnitCount),
		},
	}, nil
}

// refreshResponseToCharmhubResult converts a raw RefreshResponse from the
// charmhub API into a charmhubResult.
func (w *revisionUpdateWorker) refreshResponseToCharmhubResult(ctx context.Context, response transport.RefreshResponse) (charmhubResult, error) {
	if response.Error != nil {
		return charmhubResult{}, internalerrors.Errorf("charmhub error %s: %s", response.Error.Code, response.Error.Message)
	}

	now := w.config.Clock.Now()

	// Locate and extract the essential metadata.
	metadata, err := repository.EssentialMetadataFromResponse(response.Name, response)
	if err != nil {
		return charmhubResult{}, internalerrors.Capture(err)
	}

	channel, err := charm.ParseChannelNormalize(response.EffectiveChannel)
	if err != nil {
		return charmhubResult{}, internalerrors.Errorf("parsing effective channel %q: %w", response.EffectiveChannel, err)
	}

	// The charmhub origin is resolved from the response.
	metadata.ResolvedOrigin = corecharm.Origin{
		Source:   corecharm.CharmHub,
		Hash:     response.Entity.Download.HashSHA256,
		Type:     response.Entity.Type.String(),
		ID:       response.Entity.ID,
		Revision: &response.Entity.Revision,
		Channel:  &channel,
		// The platform is not filled in here, instead it's the same as the
		// origin platform of the application.
	}

	var resources []resource.Resource
	for _, r := range response.Entity.Resources {
		fingerprint, err := resource.ParseFingerprint(r.Download.HashSHA384)
		if err != nil {
			w.config.Logger.Warningf(ctx, "invalid resource fingerprint %q: %v", r.Download.HashSHA384, err)
			continue
		}
		typ, err := resource.ParseType(r.Type)
		if err != nil {
			w.config.Logger.Warningf(ctx, "invalid resource type %q: %v", r.Type, err)
			continue
		}
		res := resource.Resource{
			Meta: resource.Meta{
				Name:        r.Name,
				Type:        typ,
				Path:        r.Filename,
				Description: r.Description,
			},
			Origin:      resource.OriginStore,
			Revision:    r.Revision,
			Fingerprint: fingerprint,
			Size:        int64(r.Download.Size),
		}
		resources = append(resources, res)
	}
	return charmhubResult{
		name:              response.Name,
		essentialMetadata: metadata,
		timestamp:         now,
		revision:          response.Entity.Revision,
		resources:         resources,
	}, nil
}

func (w *revisionUpdateWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *revisionUpdateWorker) reportInternalState(state string) {
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}

func jitter(period time.Duration) time.Duration {
	return retry.ExpBackoff(period, period*2, 2, true)(0, 1)
}

func encodeCharmhubID(app application.RevisionUpdaterApplication, modelTag names.ModelTag) (charmhubID, error) {
	if !names.IsValidApplication(app.Name) {
		return charmhubID{}, internalerrors.Errorf("invalid application name %q", app.Name)
	}

	origin := app.Origin
	risk, err := encodeRisk(origin.Channel.Risk)
	if err != nil {
		return charmhubID{}, internalerrors.Errorf("encoding channel risk: %w", err)
	}

	channel, err := charm.MakeChannel(origin.Channel.Track, risk, origin.Channel.Branch)
	if err != nil {
		// It's actually impossible to get to the error here, as we've encoded
		// the risk above. Yet, it's better to be safe.
		return charmhubID{}, internalerrors.Errorf("making channel: %w", err)
	}

	arch, err := encodeArchitecture(origin.Platform.Architecture)
	if err != nil {
		return charmhubID{}, internalerrors.Errorf("encoding architecture: %w", err)
	}

	osType, err := encodeOSType(origin.Platform.OSType)
	if err != nil {
		return charmhubID{}, internalerrors.Errorf("encoding os type: %w", err)
	}

	return charmhubID{
		id:          origin.ID,
		revision:    origin.Revision,
		channel:     channel.String(),
		osType:      osType,
		osChannel:   origin.Platform.Channel,
		arch:        arch,
		instanceKey: charmhub.CreateInstanceKey(app.Name, modelTag),
	}, nil
}

func encodeArchitecture(a architecture.Architecture) (string, error) {
	switch a {
	case architecture.AMD64:
		return arch.AMD64, nil
	case architecture.ARM64:
		return arch.ARM64, nil
	case architecture.PPC64EL:
		return arch.PPC64EL, nil
	case architecture.S390X:
		return arch.S390X, nil
	case architecture.RISCV64:
		return arch.RISCV64, nil
	default:
		return "", internalerrors.Errorf("unsupported architecture %v", a)
	}
}

func encodeOSType(t deployment.OSType) (string, error) {
	switch t {
	case deployment.Ubuntu:
		return strings.ToLower(ostype.Ubuntu.String()), nil
	default:
		return "", internalerrors.Errorf("unsupported OS type %v", t)
	}
}

func encodeRisk(r deployment.ChannelRisk) (string, error) {
	switch r {
	case deployment.RiskStable:
		return charm.Stable.String(), nil
	case deployment.RiskCandidate:
		return charm.Candidate.String(), nil
	case deployment.RiskBeta:
		return charm.Beta.String(), nil
	case deployment.RiskEdge:
		return charm.Edge.String(), nil
	default:
		return "", internalerrors.Errorf("unsupported risk %v", r)
	}
}

type appInfo struct {
	name         string
	charmLocator applicationcharm.CharmLocator
	origin       application.Origin
}

// charmhubID holds identifying information for several charms for a
// charmhubLatestCharmInfo call.
type charmhubID struct {
	id        string
	revision  int
	channel   string
	osType    string
	osChannel string
	arch      string
	metrics   map[charmmetrics.MetricValueKey]string
	// instanceKey is a unique string associated with the application. To assist
	// with keeping KPI data in charmhub. It must be the same for every charmhub
	// Refresh action related to an application.
	instanceKey string
}

type latestCharmInfo struct {
	charmLocator      applicationcharm.CharmLocator
	essentialMetadata corecharm.EssentialMetadata
	timestamp         time.Time
	revision          int
	resources         []resource.Resource
	appName           string
}

// charmhubResult is the type charmhubLatestCharmInfo returns: information
// about a charm revision and its resources.
type charmhubResult struct {
	name              string
	essentialMetadata corecharm.EssentialMetadata
	timestamp         time.Time
	revision          int
	resources         []resource.Resource
}
