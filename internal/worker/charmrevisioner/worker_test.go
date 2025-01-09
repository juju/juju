// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisioner

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

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
	config "github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type WorkerSuite struct {
	testing.IsolationSuite

	states chan string
	now    time.Time

	modelConfigService *MockModelConfigService
	applicationService *MockApplicationService
	modelService       *MockModelService
	charmhubClient     *MockCharmhubClient
	httpClient         *MockHTTPClient
	httpClientGetter   *MockHTTPClientGetter
	clock              *MockClock
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) TestTriggerFetch(c *gc.C) {
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
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out sending time")
		}
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for fetch")
	}

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestTriggerModelConfig(c *gc.C) {
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
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out sending time")
		}
	}()

	select {
	case <-done:
	case <-time.After(testing.LongWait):
		c.Fatalf("timed out waiting for new client")
	}

	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestSendEmptyModelMetrics(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	s.expectSendEmptyModelMetrics(c)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	err := w.sendEmptyModelMetrics(context.Background(), s.charmhubClient, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *WorkerSuite) TestSendEmptyModelMetricsFails(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	s.modelService.EXPECT().GetModelMetrics(gomock.Any()).Return(coremodel.ModelMetrics{}, errors.New("boom"))

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	err := w.sendEmptyModelMetrics(context.Background(), s.charmhubClient, true)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *WorkerSuite) TestSendEmptyModelMetricsWithNoTelemetry(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	// Notice that we don't expect any call to the charmhub client.

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	err := w.sendEmptyModelMetrics(context.Background(), s.charmhubClient, false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *WorkerSuite) TestFetchInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	model := coremodel.ReadOnlyModel{
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
		id:       "foo",
		charmURL: charm.MustParseURL("ch:foo-42"),
	}}

	cfg, err := charmhub.RefreshOne(id.instanceKey, id.id, id.revision, id.channel, charmhub.RefreshBase{
		Architecture: id.arch,
		Name:         id.osType,
		Channel:      id.osChannel,
	})
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = charmhub.AddConfigMetrics(cfg, id.metrics)
	c.Assert(err, jc.ErrorIsNil)

	s.charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), charmhub.RefreshMany(cfg), metrics).Return([]transport.RefreshResponse{{
		Name: id.id,
		Entity: transport.RefreshEntity{
			Revision: 666,
		},
	}}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	result, err := w.fetchInfo(context.Background(), s.charmhubClient, true, ids, apps)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, []latestCharmInfo{{
		url:       apps[0].charmURL,
		timestamp: s.now,
		revision:  666,
		appID:     "foo",
	}})
}

func (s *WorkerSuite) TestFetchInfoInvalidResponseLength(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	model := coremodel.ReadOnlyModel{
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

	cfg, err := charmhub.RefreshOne(id.instanceKey, id.id, id.revision, id.channel, charmhub.RefreshBase{
		Architecture: id.arch,
		Name:         id.osType,
		Channel:      id.osChannel,
	})
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = charmhub.AddConfigMetrics(cfg, id.metrics)
	c.Assert(err, jc.ErrorIsNil)

	s.charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), charmhub.RefreshMany(cfg), metrics).Return([]transport.RefreshResponse{{
		Name: id.id,
		Entity: transport.RefreshEntity{
			Revision: 666,
		},
	}}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	_, err = w.fetchInfo(context.Background(), s.charmhubClient, true, ids, apps)
	c.Assert(err, gc.ErrorMatches, `expected 0 responses, got 1`)
}

func (s *WorkerSuite) TestRequest(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	model := coremodel.ReadOnlyModel{
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

	cfg, err := charmhub.RefreshOne(id.instanceKey, id.id, id.revision, id.channel, charmhub.RefreshBase{
		Architecture: id.arch,
		Name:         id.osType,
		Channel:      id.osChannel,
	})
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = charmhub.AddConfigMetrics(cfg, id.metrics)
	c.Assert(err, jc.ErrorIsNil)

	s.charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), charmhub.RefreshMany(cfg), metrics).Return([]transport.RefreshResponse{{
		Name: id.id,
	}}, nil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	result, err := w.request(context.Background(), s.charmhubClient, metrics, ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, []charmhubResult{{
		name:      id.id,
		timestamp: s.now,
	}})
}

func (s *WorkerSuite) TestRequestWithResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	model := coremodel.ReadOnlyModel{
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

	cfg, err := charmhub.RefreshOne(id.instanceKey, id.id, id.revision, id.channel, charmhub.RefreshBase{
		Architecture: id.arch,
		Name:         id.osType,
		Channel:      id.osChannel,
	})
	c.Assert(err, jc.ErrorIsNil)
	cfg, err = charmhub.AddConfigMetrics(cfg, id.metrics)
	c.Assert(err, jc.ErrorIsNil)

	hash384 := "e8e4d9727695438c7f5c91347e50e3d68eaab5fe3f856685de5a80fbaafb3c1700776dea0eb7db09c940466ba270a4e4"

	s.charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), charmhub.RefreshMany(cfg), metrics).Return([]transport.RefreshResponse{{
		Name: id.id,
		Entity: transport.RefreshEntity{
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
	c.Assert(err, jc.ErrorIsNil)

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	result, err := w.request(context.Background(), s.charmhubClient, metrics, ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, []charmhubResult{{
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
	}})
}

func (s *WorkerSuite) TestRequestWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectWatcher(c)
	s.expectModelConfig(c)

	model := coremodel.ReadOnlyModel{
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

	_, err := w.request(context.Background(), s.charmhubClient, metrics, ids)
	c.Assert(err, gc.ErrorMatches, "*api-error: boom")
}

func (s *WorkerSuite) TestEncodeCharmURL(c *gc.C) {
	url, err := encodeCharmURL(applicationcharm.CharmLocator{
		Source:       applicationcharm.CharmHubSource,
		Name:         "foo",
		Revision:     42,
		Architecture: architecture.AMD64,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(url.String(), gc.Equals, "ch:amd64/foo-42")
}

func (s *WorkerSuite) TestEncodeCharmURLLocalCharmSource(c *gc.C) {
	_, err := encodeCharmURL(applicationcharm.CharmLocator{
		Source:       applicationcharm.LocalSource,
		Name:         "foo",
		Revision:     42,
		Architecture: architecture.AMD64,
	})
	c.Assert(err, gc.ErrorMatches, `unsupported charm source "local"`)
}

func (s *WorkerSuite) TestEncodeCharmURLInvalidArchitecture(c *gc.C) {
	_, err := encodeCharmURL(applicationcharm.CharmLocator{
		Source:       applicationcharm.CharmHubSource,
		Name:         "foo",
		Revision:     42,
		Architecture: architecture.Architecture(99),
	})
	c.Assert(err, gc.ErrorMatches, `.*unsupported architecture .*`)
}

func (s *WorkerSuite) TestEncodeCharmID(c *gc.C) {
	modelTag := names.NewModelTag(uuid.MustNewUUID().String())
	id, err := encodeCharmhubID(application.RevisionUpdaterApplication{
		Name: "application-foo",
		CharmLocator: applicationcharm.CharmLocator{
			Source:       applicationcharm.LocalSource,
			Name:         "foo",
			Revision:     42,
			Architecture: architecture.AMD64,
		},
		Origin: application.Origin{
			ID: "abc123",
			Channel: application.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: application.Platform{
				Channel:      "22.04",
				OSType:       application.Ubuntu,
				Architecture: architecture.AMD64,
			},
			Revision: 42,
		},
		NumUnits: 2,
	}, modelTag)

	c.Assert(err, jc.ErrorIsNil)
	c.Check(id, gc.DeepEquals, charmhubID{
		id:          "abc123",
		revision:    42,
		channel:     "track/stable/branch",
		osType:      "ubuntu",
		osChannel:   "22.04",
		arch:        "amd64",
		metrics:     map[charmmetrics.MetricValueKey]string(nil),
		instanceKey: charmhub.CreateInstanceKey(names.NewApplicationTag("foo"), modelTag),
	})
}

func (s *WorkerSuite) TestEncodeCharmIDInvalidApplicationTag(c *gc.C) {
	modelTag := names.NewModelTag(uuid.MustNewUUID().String())
	_, err := encodeCharmhubID(application.RevisionUpdaterApplication{
		Name: "app-foo",
	}, modelTag)
	c.Assert(err, gc.ErrorMatches, `parsing application tag: "app-foo".*`)
}

func (s *WorkerSuite) TestEncodeCharmIDInvalidRisk(c *gc.C) {
	modelTag := names.NewModelTag(uuid.MustNewUUID().String())
	_, err := encodeCharmhubID(application.RevisionUpdaterApplication{
		Name: "application-foo",
		Origin: application.Origin{
			ID: "abc123",
			Channel: application.Channel{
				Track:  "track",
				Risk:   "blah",
				Branch: "branch",
			},
		},
	}, modelTag)
	c.Assert(err, gc.ErrorMatches, `encoding channel risk: unsupported risk blah`)
}

func (s *WorkerSuite) TestEncodeCharmIDInvalidArchitecture(c *gc.C) {
	modelTag := names.NewModelTag(uuid.MustNewUUID().String())
	_, err := encodeCharmhubID(application.RevisionUpdaterApplication{
		Name: "application-foo",
		Origin: application.Origin{
			ID: "abc123",
			Channel: application.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: application.Platform{
				Architecture: architecture.Architecture(99),
			},
		},
	}, modelTag)
	c.Assert(err, gc.ErrorMatches, `encoding architecture: .*`)
}

func (s *WorkerSuite) TestEncodeCharmIDInvalidOSType(c *gc.C) {
	modelTag := names.NewModelTag(uuid.MustNewUUID().String())
	_, err := encodeCharmhubID(application.RevisionUpdaterApplication{
		Name: "application-foo",
		Origin: application.Origin{
			ID: "abc123",
			Channel: application.Channel{
				Track:  "track",
				Risk:   "stable",
				Branch: "branch",
			},
			Platform: application.Platform{
				Architecture: architecture.AMD64,
				OSType:       application.OSType(99),
			},
		},
	}, modelTag)
	c.Assert(err, gc.ErrorMatches, `encoding os type: .*`)
}

func (s *WorkerSuite) TestEncodeArchitecture(c *gc.C) {
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
		c.Assert(err, jc.ErrorIsNil)
		c.Check(got, gc.Equals, test.expected)
	}
}

func (s *WorkerSuite) TestEncodeRisk(c *gc.C) {
	tests := []struct {
		value    application.ChannelRisk
		expected string
	}{
		{
			value:    application.RiskStable,
			expected: "stable",
		},
		{
			value:    application.RiskCandidate,
			expected: "candidate",
		},
		{
			value:    application.RiskBeta,
			expected: "beta",
		},
		{
			value:    application.RiskEdge,
			expected: "edge",
		},
	}

	for i, test := range tests {
		c.Logf("test %d", i)

		got, err := encodeRisk(test.value)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(got, gc.Equals, test.expected)
	}
}

func (s *WorkerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.charmhubClient = NewMockCharmhubClient(ctrl)

	s.now = time.Now()

	s.clock = NewMockClock(ctrl)
	s.clock.EXPECT().Now().Return(s.now).AnyTimes()

	return ctrl
}

func (s *WorkerSuite) newWorker(c *gc.C) *revisionUpdateWorker {
	w, err := newWorker(Config{
		ModelConfigService: s.modelConfigService,
		ApplicationService: s.applicationService,
		ModelService:       s.modelService,
		ModelTag:           names.NewModelTag(uuid.MustNewUUID().String()),
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
	c.Assert(err, jc.ErrorIsNil)
	return w.(*revisionUpdateWorker)
}

func (s *WorkerSuite) expectWatcher(c *gc.C) {
	ch := make(chan []string)
	watcher := watchertest.NewMockStringsWatcher(ch)
	s.modelConfigService.EXPECT().Watch().Return(watcher, nil)
	s.clock.EXPECT().After(gomock.Any()).DoAndReturn(func(d time.Duration) <-chan time.Time {
		return nil
	})
}

func (s *WorkerSuite) expectModelConfig(c *gc.C) {
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(&config.Config{}, nil)
}

func (s *WorkerSuite) expectSendEmptyModelMetrics(c *gc.C) {
	model := coremodel.ReadOnlyModel{
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

func (s *WorkerSuite) ensureStartup(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}
