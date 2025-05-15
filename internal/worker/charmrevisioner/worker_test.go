// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisioner

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/charm"
	charmmetrics "github.com/juju/juju/core/charm/metrics"
	http "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	domainresource "github.com/juju/juju/domain/resource"
	config "github.com/juju/juju/environs/config"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type WorkerSuite struct {
	testhelpers.IsolationSuite

	states chan string
	now    time.Time

	modelConfigService *MockModelConfigService
	applicationService *MockApplicationService
	modelService       *MockModelService
	resourceService    *MockResourceService
	charmhubClient     *MockCharmhubClient
	httpClient         *MockHTTPClient
	httpClientGetter   *MockHTTPClientGetter
	clock              *MockClock

	modelTag names.ModelTag
}

var _ = tc.Suite(&WorkerSuite{})

func (s *WorkerSuite) TestTriggerFetch(c *tc.C) {
	// Ensure that a clock tick triggers a fetch, the testing of the fetch
	// is done in other methods.

	defer s.setupMocks(c).Finish()

	watcher := watchertest.NewMockStringsWatcher(make(chan []string))
	s.modelConfigService.EXPECT().Watch().Return(watcher, nil)

	ch := make(chan time.Time)

	// These are required to be in-order.
	gomock.InOrder(
		s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
			return ch
		}),
		s.clock.EXPECT().After(gomock.Any()).Return(make(<-chan time.Time)),
	)

	done := make(chan struct{})
	s.applicationService.EXPECT().GetApplicationsForRevisionUpdater(gomock.Any()).DoAndReturn(func(ctx context.Context) ([]application.RevisionUpdaterApplication, error) {
		close(done)
		return nil, nil
	})

	s.expectModelConfig(c)
	s.expectModelConfig(c)
	s.expectSendEmptyModelMetrics(c)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	go func() {
		select {
		case ch <- time.Now():
		case <-time.After(testhelpers.LongWait):
			c.Fatalf("timed out sending time")
		}
	}()

	select {
	case <-done:
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("timed out waiting for fetch")
	}

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestTriggerModelConfig(c *tc.C) {
	// Ensure that a model config change triggers a new charmhub client.
	defer s.setupMocks(c).Finish()

	// This will block, and we can then trigger the model config change
	// independently.
	s.clock.EXPECT().After(gomock.Any()).Return(make(<-chan time.Time)).AnyTimes()

	ch := make(chan []string)
	watcher := watchertest.NewMockStringsWatcher(ch)
	s.modelConfigService.EXPECT().Watch().Return(watcher, nil)

	done := make(chan struct{})

	// The first model config request is for the initial client, the second
	// one is for the new client change.
	gomock.InOrder(
		s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(&config.Config{}, nil),
		s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).DoAndReturn(func(ctx context.Context) (*config.Config, error) {
			close(done)
			return &config.Config{}, nil
		}),
	)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	go func() {
		select {
		case ch <- []string{config.CharmHubURLKey}:
		case <-time.After(testhelpers.LongWait):
			c.Fatalf("timed out sending time")
		}
	}()

	select {
	case <-done:
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("timed out waiting for new client")
	}

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestSendEmptyModelMetrics(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	s.expectSendEmptyModelMetrics(c)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	err := w.sendEmptyModelMetrics(c.Context(), s.charmhubClient, true)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *WorkerSuite) TestSendEmptyModelMetricsFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	s.modelService.EXPECT().GetModelMetrics(gomock.Any()).Return(coremodel.ModelMetrics{}, errors.New("boom"))

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	err := w.sendEmptyModelMetrics(c.Context(), s.charmhubClient, true)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *WorkerSuite) TestSendEmptyModelMetricsWithNoTelemetry(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	// Notice that we don't expect any call to the charmhub client.

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	err := w.sendEmptyModelMetrics(c.Context(), s.charmhubClient, false)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *WorkerSuite) TestFetch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)
	s.expectModelConfig(c)

	charmLocator := applicationcharm.CharmLocator{
		Source:       applicationcharm.CharmHubSource,
		Name:         "foo",
		Revision:     42,
		Architecture: architecture.AMD64,
	}

	s.applicationService.EXPECT().GetApplicationsForRevisionUpdater(gomock.Any()).DoAndReturn(func(ctx context.Context) ([]application.RevisionUpdaterApplication, error) {
		return []application.RevisionUpdaterApplication{{
			Name:         "foo",
			CharmLocator: charmLocator,
			Origin: application.Origin{
				ID:       "foo",
				Revision: 42,
				Channel: deployment.Channel{
					Risk: "stable",
				},
				Platform: deployment.Platform{
					Architecture: architecture.AMD64,
					Channel:      "22.04",
					OSType:       deployment.Ubuntu,
				},
			},
		}}, nil
	})

	model := coremodel.ModelInfo{
		UUID:           modeltesting.GenModelUUID(c),
		ControllerUUID: uuid.MustNewUUID(),
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "us-east-1",
	}

	s.modelService.EXPECT().GetModelMetrics(gomock.Any()).Return(coremodel.ModelMetrics{
		Model:            model,
		ApplicationCount: 1,
		MachineCount:     2,
		UnitCount:        3,
	}, nil)

	s.charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{{
		Name:             "foo",
		EffectiveChannel: "latest/stable",
		Entity: transport.RefreshEntity{
			Revision: 666,
		},
	}}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	result, err := w.fetch(c.Context(), s.charmhubClient)
	c.Assert(err, tc.ErrorIsNil)

	channel := internalcharm.MakePermissiveChannel("latest", "stable", "")
	c.Check(result, tc.DeepEquals, []latestCharmInfo{{
		essentialMetadata: charm.EssentialMetadata{
			ResolvedOrigin: charm.Origin{
				Source:   charm.CharmHub,
				Revision: ptr(666),
				Channel:  &channel,
				Platform: charm.Platform{
					Architecture: "amd64",
					OS:           "ubuntu",
					Channel:      "22.04",
				},
			},
		},
		charmLocator: charmLocator,
		timestamp:    s.now,
		revision:     666,
		appName:      "foo",
	}})
}

func (s *WorkerSuite) TestFetchInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	model := coremodel.ModelInfo{
		UUID:           modeltesting.GenModelUUID(c),
		ControllerUUID: uuid.MustNewUUID(),
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "us-east-1",
	}
	metrics := charmhub.Metrics{
		charmmetrics.Controller: {
			charmmetrics.JujuVersion: version.Current.String(),
			charmmetrics.UUID:        model.ControllerUUID.String(),
		},
		charmmetrics.Model: {
			charmmetrics.UUID:     model.UUID.String(),
			charmmetrics.Cloud:    model.Cloud,
			charmmetrics.Provider: model.CloudType,
			charmmetrics.Region:   model.CloudRegion,

			charmmetrics.NumApplications: "1",
			charmmetrics.NumMachines:     "2",
			charmmetrics.NumUnits:        "3",
		},
	}

	s.modelService.EXPECT().GetModelMetrics(gomock.Any()).Return(coremodel.ModelMetrics{
		Model:            model,
		ApplicationCount: 1,
		MachineCount:     2,
		UnitCount:        3,
	}, nil)

	ids := []charmhubID{{
		id:          "foo",
		revision:    42,
		channel:     "stable",
		osType:      "ubuntu",
		osChannel:   "22.04",
		arch:        "amd64",
		metrics:     metrics[charmmetrics.Model],
		instanceKey: "abc123",
	}}
	id := ids[0]

	apps := []appInfo{{
		name: "foo",
		charmLocator: applicationcharm.CharmLocator{
			Source:       applicationcharm.CharmHubSource,
			Name:         "foo",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
	}}

	cfg, err := charmhub.RefreshOne(context.Background(), id.instanceKey, id.id, id.revision, id.channel, charmhub.RefreshBase{
		Architecture: id.arch,
		Name:         id.osType,
		Channel:      id.osChannel,
	})
	c.Assert(err, tc.ErrorIsNil)
	cfg, err = charmhub.AddConfigMetrics(cfg, id.metrics)
	c.Assert(err, tc.ErrorIsNil)

	s.charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), charmhub.RefreshMany(cfg), metrics).Return([]transport.RefreshResponse{{
		Name:             id.id,
		EffectiveChannel: "latest/stable",
		Entity: transport.RefreshEntity{
			Revision: 666,
		},
	}}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	result, err := w.fetchInfo(c.Context(), s.charmhubClient, true, ids, apps)
	c.Assert(err, tc.ErrorIsNil)

	channel := internalcharm.MakePermissiveChannel("latest", "stable", "")
	c.Check(result, tc.DeepEquals, []latestCharmInfo{{
		essentialMetadata: charm.EssentialMetadata{
			ResolvedOrigin: charm.Origin{
				Source:   charm.CharmHub,
				Revision: ptr(666),
				Channel:  &channel,
				Platform: charm.Platform{
					Architecture: "amd64",
					OS:           "ubuntu",
				},
			},
		},
		charmLocator: apps[0].charmLocator,
		timestamp:    s.now,
		revision:     666,
		appName:      "foo",
	}})
}

func (s *WorkerSuite) TestFetchInfoInvalidResponseLength(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	model := coremodel.ModelInfo{
		UUID:           modeltesting.GenModelUUID(c),
		ControllerUUID: uuid.MustNewUUID(),
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "us-east-1",
	}
	metrics := charmhub.Metrics{
		charmmetrics.Controller: {
			charmmetrics.JujuVersion: version.Current.String(),
			charmmetrics.UUID:        model.ControllerUUID.String(),
		},
		charmmetrics.Model: {
			charmmetrics.UUID:     model.UUID.String(),
			charmmetrics.Cloud:    model.Cloud,
			charmmetrics.Provider: model.CloudType,
			charmmetrics.Region:   model.CloudRegion,

			charmmetrics.NumApplications: "1",
			charmmetrics.NumMachines:     "2",
			charmmetrics.NumUnits:        "3",
		},
	}

	s.modelService.EXPECT().GetModelMetrics(gomock.Any()).Return(coremodel.ModelMetrics{
		Model:            model,
		ApplicationCount: 1,
		MachineCount:     2,
		UnitCount:        3,
	}, nil)

	ids := []charmhubID{{
		id:          "foo",
		revision:    42,
		channel:     "stable",
		osType:      "ubuntu",
		osChannel:   "22.04",
		arch:        "amd64",
		metrics:     metrics[charmmetrics.Model],
		instanceKey: "abc123",
	}}
	id := ids[0]

	apps := []appInfo{}

	cfg, err := charmhub.RefreshOne(context.Background(), id.instanceKey, id.id, id.revision, id.channel, charmhub.RefreshBase{
		Architecture: id.arch,
		Name:         id.osType,
		Channel:      id.osChannel,
	})
	c.Assert(err, tc.ErrorIsNil)
	cfg, err = charmhub.AddConfigMetrics(cfg, id.metrics)
	c.Assert(err, tc.ErrorIsNil)

	s.charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), charmhub.RefreshMany(cfg), metrics).Return([]transport.RefreshResponse{{
		Name:             id.id,
		EffectiveChannel: "latest/stable",
		Entity: transport.RefreshEntity{
			Revision: 666,
		},
	}}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	_, err = w.fetchInfo(c.Context(), s.charmhubClient, true, ids, apps)
	c.Assert(err, tc.ErrorMatches, `expected 0 responses, got 1`)
}

func (s *WorkerSuite) TestRequest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	model := coremodel.ModelInfo{
		UUID:           modeltesting.GenModelUUID(c),
		ControllerUUID: uuid.MustNewUUID(),
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "us-east-1",
	}
	metrics := charmhub.Metrics{
		charmmetrics.Controller: {
			charmmetrics.JujuVersion: version.Current.String(),
			charmmetrics.UUID:        model.ControllerUUID.String(),
		},
		charmmetrics.Model: {
			charmmetrics.UUID:     model.UUID.String(),
			charmmetrics.Cloud:    model.Cloud,
			charmmetrics.Provider: model.CloudType,
			charmmetrics.Region:   model.CloudRegion,

			charmmetrics.NumApplications: "1",
			charmmetrics.NumMachines:     "2",
			charmmetrics.NumUnits:        "3",
		},
	}

	ids := []charmhubID{{
		id:          "foo",
		revision:    42,
		channel:     "stable",
		osType:      "ubuntu",
		osChannel:   "22.04",
		arch:        "amd64",
		metrics:     metrics[charmmetrics.Model],
		instanceKey: "abc123",
	}}
	id := ids[0]

	cfg, err := charmhub.RefreshOne(context.Background(), id.instanceKey, id.id, id.revision, id.channel, charmhub.RefreshBase{
		Architecture: id.arch,
		Name:         id.osType,
		Channel:      id.osChannel,
	})
	c.Assert(err, tc.ErrorIsNil)
	cfg, err = charmhub.AddConfigMetrics(cfg, id.metrics)
	c.Assert(err, tc.ErrorIsNil)

	s.charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), charmhub.RefreshMany(cfg), metrics).Return([]transport.RefreshResponse{{
		Name:             id.id,
		EffectiveChannel: "latest/stable",
		Entity: transport.RefreshEntity{
			Revision: 666,
		},
	}}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	channel := internalcharm.MakePermissiveChannel("latest", "stable", "")
	result, err := w.request(c.Context(), s.charmhubClient, metrics, ids)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []charmhubResult{{
		name:      id.id,
		timestamp: s.now,
		essentialMetadata: charm.EssentialMetadata{
			ResolvedOrigin: charm.Origin{
				Source:   charm.CharmHub,
				Revision: ptr(666),
				Channel:  &channel,
			},
		},
		revision: 666,
	}})
}

func (s *WorkerSuite) TestRequestWithResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	model := coremodel.ModelInfo{
		UUID:           modeltesting.GenModelUUID(c),
		ControllerUUID: uuid.MustNewUUID(),
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "us-east-1",
	}
	metrics := charmhub.Metrics{
		charmmetrics.Controller: {
			charmmetrics.JujuVersion: version.Current.String(),
			charmmetrics.UUID:        model.ControllerUUID.String(),
		},
		charmmetrics.Model: {
			charmmetrics.UUID:     model.UUID.String(),
			charmmetrics.Cloud:    model.Cloud,
			charmmetrics.Provider: model.CloudType,
			charmmetrics.Region:   model.CloudRegion,

			charmmetrics.NumApplications: "1",
			charmmetrics.NumMachines:     "2",
			charmmetrics.NumUnits:        "3",
		},
	}

	ids := []charmhubID{{
		id:          "foo",
		revision:    42,
		channel:     "stable",
		osType:      "ubuntu",
		osChannel:   "22.04",
		arch:        "amd64",
		metrics:     metrics[charmmetrics.Model],
		instanceKey: "abc123",
	}}
	id := ids[0]

	cfg, err := charmhub.RefreshOne(context.Background(), id.instanceKey, id.id, id.revision, id.channel, charmhub.RefreshBase{
		Architecture: id.arch,
		Name:         id.osType,
		Channel:      id.osChannel,
	})
	c.Assert(err, tc.ErrorIsNil)
	cfg, err = charmhub.AddConfigMetrics(cfg, id.metrics)
	c.Assert(err, tc.ErrorIsNil)

	hash384 := "e8e4d9727695438c7f5c91347e50e3d68eaab5fe3f856685de5a80fbaafb3c1700776dea0eb7db09c940466ba270a4e4"

	s.charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), charmhub.RefreshMany(cfg), metrics).Return([]transport.RefreshResponse{{
		Name:             id.id,
		EffectiveChannel: "latest/stable",
		Entity: transport.RefreshEntity{
			Revision: 666,
			Resources: []transport.ResourceRevision{{
				Type:     "file",
				Revision: 42,
				Download: transport.Download{
					HashSHA384: hash384,
				},
			}},
		},
	}}, nil)

	fingerprint, err := resource.ParseFingerprint(hash384)
	c.Assert(err, tc.ErrorIsNil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	channel := internalcharm.MakePermissiveChannel("latest", "stable", "")
	result, err := w.request(c.Context(), s.charmhubClient, metrics, ids)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, []charmhubResult{{
		name:      id.id,
		timestamp: s.now,
		resources: []resource.Resource{{
			Meta: resource.Meta{
				Type: resource.TypeFile,
			},
			Origin:      resource.OriginStore,
			Fingerprint: fingerprint,
			Revision:    42,
		}},
		essentialMetadata: charm.EssentialMetadata{
			ResolvedOrigin: charm.Origin{
				Source:   charm.CharmHub,
				Revision: ptr(666),
				Channel:  &channel,
			},
		},
		revision: 666,
	}})
}

func (s *WorkerSuite) TestRequestWithError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	model := coremodel.ModelInfo{
		UUID:           modeltesting.GenModelUUID(c),
		ControllerUUID: uuid.MustNewUUID(),
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "us-east-1",
	}
	metrics := charmhub.Metrics{
		charmmetrics.Controller: {
			charmmetrics.JujuVersion: version.Current.String(),
			charmmetrics.UUID:        model.ControllerUUID.String(),
		},
		charmmetrics.Model: {
			charmmetrics.UUID:     model.UUID.String(),
			charmmetrics.Cloud:    model.Cloud,
			charmmetrics.Provider: model.CloudType,
			charmmetrics.Region:   model.CloudRegion,

			charmmetrics.NumApplications: "1",
			charmmetrics.NumMachines:     "2",
			charmmetrics.NumUnits:        "3",
		},
	}

	ids := []charmhubID{{
		id:          "foo",
		revision:    42,
		channel:     "stable",
		osType:      "ubuntu",
		osChannel:   "22.04",
		arch:        "amd64",
		metrics:     metrics[charmmetrics.Model],
		instanceKey: "abc123",
	}}

	s.charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{{
		Error: &transport.APIError{
			Code:    transport.ErrorCodeAPIError,
			Message: "boom",
		},
	}}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	_, err := w.request(c.Context(), s.charmhubClient, metrics, ids)
	c.Assert(err, tc.ErrorMatches, "*api-error: boom")
}

func (s *WorkerSuite) TestStoreNewRevisionsNoUpdates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	latestCharmInfos := []latestCharmInfo{}

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	err := w.storeNewRevisions(c.Context(), latestCharmInfos)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *WorkerSuite) TestStoreNewCharmRevisionsNoResource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	latestCharmInfos := []latestCharmInfo{{
		charmLocator: applicationcharm.CharmLocator{
			Source:       applicationcharm.CharmHubSource,
			Name:         "foo",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
		essentialMetadata: charm.EssentialMetadata{
			Meta: &internalcharm.Meta{
				Name: "foo",
			},
			Manifest: &internalcharm.Manifest{
				Bases: []internalcharm.Base{{
					Name: "ubuntu",
					Channel: internalcharm.Channel{
						Risk: "stable",
					},
					Architectures: []string{"amd64"},
				}},
			},
			DownloadInfo: charm.DownloadInfo{
				CharmhubIdentifier: "abc123",
				DownloadURL:        "https://example.com/foo",
				DownloadSize:       123,
			},
		},
		timestamp: s.now,
		revision:  43,
		appName:   "foo",
	}}
	essentialMetadata := latestCharmInfos[0].essentialMetadata

	s.applicationService.EXPECT().ReserveCharmRevision(gomock.Any(), applicationcharm.ReserveCharmRevisionArgs{
		Charm: internalcharm.NewCharmBase(
			essentialMetadata.Meta,
			essentialMetadata.Manifest,
			essentialMetadata.Config,
			// These will be filled in once we have all the data in the
			// response from the charmhub.
			nil, nil,
		),

		Source:        charm.CharmHub,
		ReferenceName: "foo",
		Revision:      43,
		DownloadInfo: &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceDownload,
			CharmhubIdentifier: "abc123",
			DownloadURL:        "https://example.com/foo",
			DownloadSize:       123,
		},
	}).Return(charm.ID("foo"), nil, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	err := w.storeNewRevisions(c.Context(), latestCharmInfos)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *WorkerSuite) TestStoreNewCharmRevisionsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	latestCharmInfos := []latestCharmInfo{{
		charmLocator: applicationcharm.CharmLocator{
			Source:       applicationcharm.CharmHubSource,
			Name:         "foo",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
	}}

	s.applicationService.EXPECT().ReserveCharmRevision(gomock.Any(), gomock.Any()).Return(charm.ID("foo"), nil, errors.New("boom"))

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	err := w.storeNewRevisions(c.Context(), latestCharmInfos)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *WorkerSuite) TestStoreNewResourceRevisions(c *tc.C) {
	// Arrange: create two application with new resources from charm
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	latestCharmInfos := []latestCharmInfo{{
		appName: "foo",
		resources: []resource.Resource{{
			Meta:     resource.Meta{Name: "foo"},
			Origin:   resource.OriginStore,
			Revision: 42,
		}},
	}, {
		appName: "bar",
		resources: []resource.Resource{{
			Meta:     resource.Meta{Name: "bar-1"},
			Origin:   resource.OriginStore,
			Revision: 42,
		}, {
			Meta:     resource.Meta{Name: "bar-2"},
			Origin:   resource.OriginStore,
			Revision: 24,
		}},
	}}

	// This function is called but won't contribute to the revisions storage
	s.applicationService.EXPECT().ReserveCharmRevision(gomock.Any(), gomock.Any()).Return("foo-charm-id", nil, nil).AnyTimes()

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("foo-id", nil)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "bar").Return("bar-id", nil)
	s.resourceService.EXPECT().SetRepositoryResources(gomock.Any(), domainresource.SetRepositoryResourcesArgs{
		ApplicationID: "foo-id",
		CharmID:       "foo-charm-id",
		Info:          latestCharmInfos[0].resources,
		LastPolled:    s.now,
	}).Return(nil)
	s.resourceService.EXPECT().SetRepositoryResources(gomock.Any(), domainresource.SetRepositoryResourcesArgs{
		ApplicationID: "bar-id",
		CharmID:       "foo-charm-id",
		Info:          latestCharmInfos[1].resources,
		LastPolled:    s.now,
	}).Return(nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Act: run worker function
	err := w.storeNewRevisions(c.Context(), latestCharmInfos)

	// Assert: real assertion are done in what as been called in the mock
	c.Assert(err, tc.ErrorIsNil)
}

func (s *WorkerSuite) TestStoreNewResourceRevisionsWithApplicationNotFound(c *tc.C) {
	// Arrange: create two application with new resources from charm
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	latestCharmInfos := []latestCharmInfo{
		{
			appName: "not-found",
			// Resources need to not be empty: if empty, SetRepositoryResources
			// won't be triggered
			resources: []resource.Resource{{}},
		},
		{
			appName: "foo",
			resources: []resource.Resource{{
				Meta:     resource.Meta{Name: "foo"},
				Origin:   resource.OriginStore,
				Revision: 42,
			}},
		}}

	// This function is called but won't contribute to the revisions storage
	s.applicationService.EXPECT().ReserveCharmRevision(gomock.Any(), gomock.Any()).Return("foo-charm-id", nil,
		nil).AnyTimes()

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "not-found").Return("",
		applicationerrors.ApplicationNotFound)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("foo-id", nil)
	s.resourceService.EXPECT().SetRepositoryResources(gomock.Any(), domainresource.SetRepositoryResourcesArgs{
		ApplicationID: "foo-id",
		CharmID:       "foo-charm-id",
		Info:          latestCharmInfos[1].resources,
		LastPolled:    s.now,
	}).Return(nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Act: run worker function
	err := w.storeNewRevisions(c.Context(), latestCharmInfos)

	// Assert: check everything ok through mocks, but also that a log as been
	// generated for the unknown application.
	c.Assert(err, tc.ErrorIsNil)
	//c.Check(c.GetTestLog(), tc.Contains, `failed to get application ID for "not-found"`)
}

func (s *WorkerSuite) TestStoreNewResourceRevisionsErrorGetApplicationID(c *tc.C) {
	// Arrange: create two application with new resources from charm
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	latestCharmInfos := []latestCharmInfo{
		{
			appName: "foo",
			// Resources need to not be empty: if empty, SetRepositoryResources
			// won't be triggered
			resources: []resource.Resource{{}},
		}}
	expectedError := errors.New("boom")

	// This function is called but won't contribute to the revisions storage
	s.applicationService.EXPECT().ReserveCharmRevision(gomock.Any(), gomock.Any()).Return("foo", nil, nil).AnyTimes()

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("",
		expectedError)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Act: run worker function
	err := w.storeNewRevisions(c.Context(), latestCharmInfos)

	// Assert: Check the error is returned
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *WorkerSuite) TestStoreNewResourceRevisionsErrorStoreRepositoryResources(c *tc.C) {
	// Arrange: create two application with new resources from charm
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	latestCharmInfos := []latestCharmInfo{
		{
			appName: "foo",
			// Resources need to not be empty: if empty, SetRepositoryResources
			// won't be triggered
			resources: []resource.Resource{{}},
		}}
	expectedError := errors.New("boom")

	// This function is called but won't contribute to the revisions storage
	s.applicationService.EXPECT().ReserveCharmRevision(gomock.Any(), gomock.Any()).Return("foo", nil, nil).AnyTimes()

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("foo-id", nil)
	s.resourceService.EXPECT().SetRepositoryResources(gomock.Any(), gomock.Any()).Return(expectedError)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Act: run worker function
	err := w.storeNewRevisions(c.Context(), latestCharmInfos)

	// Assert: Check the error is returned
	c.Assert(err, tc.ErrorIs, expectedError)
}

func (s *WorkerSuite) TestEncodeCharmID(c *tc.C) {
	modelTag := names.NewModelTag(uuid.MustNewUUID().String())
	id, err := encodeCharmhubID(application.RevisionUpdaterApplication{
		Name: "foo",
		CharmLocator: applicationcharm.CharmLocator{
			Source:       applicationcharm.LocalSource,
			Name:         "foo",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
		Origin: application.Origin{
			ID: "abc123",
			Channel: deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: deployment.Platform{
				Channel:      "22.04",
				OSType:       deployment.Ubuntu,
				Architecture: architecture.AMD64,
			},
			Revision: 42,
		},
		NumUnits: 2,
	}, modelTag)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(id, tc.DeepEquals, charmhubID{
		id:          "abc123",
		revision:    42,
		channel:     "track/stable/branch",
		osType:      "ubuntu",
		osChannel:   "22.04",
		arch:        "amd64",
		metrics:     map[charmmetrics.MetricValueKey]string(nil),
		instanceKey: charmhub.CreateInstanceKey("foo", modelTag),
	})
}

func (s *WorkerSuite) TestEncodeCharmIDInvalidApplicationTag(c *tc.C) {
	modelTag := names.NewModelTag(uuid.MustNewUUID().String())
	_, err := encodeCharmhubID(application.RevisionUpdaterApplication{
		Name: "!foo",
	}, modelTag)
	c.Assert(err, tc.ErrorMatches, `invalid application name "!foo"`)
}

func (s *WorkerSuite) TestEncodeCharmIDInvalidRisk(c *tc.C) {
	modelTag := names.NewModelTag(uuid.MustNewUUID().String())
	_, err := encodeCharmhubID(application.RevisionUpdaterApplication{
		Name: "application-foo",
		Origin: application.Origin{
			ID: "abc123",
			Channel: deployment.Channel{
				Track:  "track",
				Risk:   "blah",
				Branch: "branch",
			},
		},
	}, modelTag)
	c.Assert(err, tc.ErrorMatches, `encoding channel risk: unsupported risk blah`)
}

func (s *WorkerSuite) TestEncodeCharmIDInvalidArchitecture(c *tc.C) {
	modelTag := names.NewModelTag(uuid.MustNewUUID().String())
	_, err := encodeCharmhubID(application.RevisionUpdaterApplication{
		Name: "application-foo",
		Origin: application.Origin{
			ID: "abc123",
			Channel: deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: deployment.Platform{
				Architecture: architecture.Architecture(99),
			},
		},
	}, modelTag)
	c.Assert(err, tc.ErrorMatches, `encoding architecture: .*`)
}

func (s *WorkerSuite) TestEncodeCharmIDInvalidOSType(c *tc.C) {
	modelTag := names.NewModelTag(uuid.MustNewUUID().String())
	_, err := encodeCharmhubID(application.RevisionUpdaterApplication{
		Name: "application-foo",
		Origin: application.Origin{
			ID: "abc123",
			Channel: deployment.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: deployment.Platform{
				Architecture: architecture.AMD64,
				OSType:       deployment.OSType(99),
			},
		},
	}, modelTag)
	c.Assert(err, tc.ErrorMatches, `encoding os type: .*`)
}

func (s *WorkerSuite) TestEncodeArchitecture(c *tc.C) {
	tests := []struct {
		value    architecture.Architecture
		expected string
	}{
		{
			value:    architecture.AMD64,
			expected: "amd64",
		},
		{
			value:    architecture.ARM64,
			expected: "arm64",
		},
		{
			value:    architecture.PPC64EL,
			expected: "ppc64el",
		},
		{
			value:    architecture.RISCV64,
			expected: "riscv64",
		},
		{
			value:    architecture.S390X,
			expected: "s390x",
		},
	}

	for i, test := range tests {
		c.Logf("test %d", i)

		got, err := encodeArchitecture(test.value)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(got, tc.Equals, test.expected)
	}
}

func (s *WorkerSuite) TestEncodeRisk(c *tc.C) {
	tests := []struct {
		value    deployment.ChannelRisk
		expected string
	}{
		{
			value:    deployment.RiskStable,
			expected: "stable",
		},
		{
			value:    deployment.RiskCandidate,
			expected: "candidate",
		},
		{
			value:    deployment.RiskBeta,
			expected: "beta",
		},
		{
			value:    deployment.RiskEdge,
			expected: "edge",
		},
	}

	for i, test := range tests {
		c.Logf("test %d", i)

		got, err := encodeRisk(test.value)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(got, tc.Equals, test.expected)
	}
}

func (s *WorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.resourceService = NewMockResourceService(ctrl)
	s.charmhubClient = NewMockCharmhubClient(ctrl)

	s.now = time.Now()

	s.clock = NewMockClock(ctrl)
	s.clock.EXPECT().Now().Return(s.now).AnyTimes()

	s.modelTag = names.NewModelTag(uuid.MustNewUUID().String())

	return ctrl
}

func (s *WorkerSuite) newWorker(c *tc.C) *revisionUpdateWorker {
	w, err := newWorker(Config{
		ModelConfigService: s.modelConfigService,
		ApplicationService: s.applicationService,
		ModelService:       s.modelService,
		ResourceService:    s.resourceService,
		ModelTag:           s.modelTag,
		NewHTTPClient: func(context.Context, http.HTTPClientGetter) (http.HTTPClient, error) {
			return s.httpClient, nil
		},
		HTTPClientGetter: s.httpClientGetter,
		NewCharmhubClient: func(charmhub.HTTPClient, string, logger.Logger) (CharmhubClient, error) {
			return s.charmhubClient, nil
		},
		Period: time.Second,
		Clock:  s.clock,
		Logger: loggertesting.WrapCheckLog(c),
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	return w.(*revisionUpdateWorker)
}

func (s *WorkerSuite) expectWatcher(c *tc.C) {
	ch := make(chan []string)
	watcher := watchertest.NewMockStringsWatcher(ch)
	s.modelConfigService.EXPECT().Watch().Return(watcher, nil)
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
		return nil
	})
}

func (s *WorkerSuite) expectModelConfig(c *tc.C) {
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(&config.Config{}, nil)
}

func (s *WorkerSuite) expectSendEmptyModelMetrics(c *tc.C) {
	model := coremodel.ModelInfo{
		UUID:           modeltesting.GenModelUUID(c),
		ControllerUUID: uuid.MustNewUUID(),
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "us-east-1",
	}

	s.modelService.EXPECT().GetModelMetrics(gomock.Any()).Return(coremodel.ModelMetrics{
		Model:            model,
		ApplicationCount: 1,
		MachineCount:     2,
		UnitCount:        3,
	}, nil)

	metrics := charmhub.Metrics{
		charmmetrics.Controller: {
			charmmetrics.JujuVersion: version.Current.String(),
			charmmetrics.UUID:        model.ControllerUUID.String(),
		},
		charmmetrics.Model: {
			charmmetrics.UUID:     model.UUID.String(),
			charmmetrics.Cloud:    model.Cloud,
			charmmetrics.Provider: model.CloudType,
			charmmetrics.Region:   model.CloudRegion,

			charmmetrics.NumApplications: "1",
			charmmetrics.NumMachines:     "2",
			charmmetrics.NumUnits:        "3",
		},
	}

	s.charmhubClient.EXPECT().RefreshWithMetricsOnly(gomock.Any(), metrics).Return(nil)
}

func (s *WorkerSuite) ensureStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-time.After(testhelpers.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}
