// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
)

type trackerWorkerSuite struct {
	baseSuite
}

var _ = tc.Suite(&trackerWorkerSuite{})

func (s *trackerWorkerSuite) TestWorkerStartup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// Create the worker.

	w, err := s.newWorker(c, s.environ)
	c.Assert(err, tc.ErrorIsNil)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)
}

func (s *trackerWorkerSuite) TestWorkerStartupWithCloudSpec(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with the cloud spec setter and environ.

	uuid := s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)

	// Now we've got the cloud spec setter, we need to ensure we watch the
	// cloud and credentials.

	s.expectModelCloudCredentialWatcher(c, uuid)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// Create the worker.

	w, err := s.newWorker(c, s.newCloudSpecEnviron())
	c.Assert(err, tc.ErrorIsNil)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)
}

func (s *trackerWorkerSuite) TestWorkerModelConfigUpdatesEnviron(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	ch := s.expectConfigWatcher(c)
	s.expectEnvironSetConfig(c, cfg)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// Create the worker.

	w, err := s.newWorker(c, s.environ)
	c.Assert(err, tc.ErrorIsNil)

	s.ensureStartup(c)

	// Dispatch a config change, ensure it's picked up via the environ.

	select {
	case ch <- []string{"foo"}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out sending config change")
	}

	workertest.CleanKill(c, w)
}

func (s *trackerWorkerSuite) TestWorkerCloudUpdatesEnviron(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	uuid := s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)

	// Now we've got the cloud spec setter, we need to ensure we watch the
	// cloud and credentials.

	ch := s.expectModelCloudCredentialWatcher(c, uuid)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// This will cause the cloud spec to be updated.
	s.expectEnvironSetSpecUpdate(c)

	// Create the worker.

	w, err := s.newWorker(c, s.newCloudSpecEnviron())
	c.Assert(err, tc.ErrorIsNil)

	s.ensureStartup(c)

	// Send a notification so that a cloud change is picked up.

	select {
	case ch <- struct{}{}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out sending config change")
	}

	workertest.CleanKill(c, w)
}

func (s *trackerWorkerSuite) TestWorkerCredentialUpdatesEnviron(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	uuid := s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)

	// Now we've got the cloud spec setter, we need to ensure we watch the
	// cloud and credentials.

	ch := s.expectModelCloudCredentialWatcher(c, uuid)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// This will cause the cloud spec to be updated.
	s.expectEnvironSetSpecUpdate(c)

	// Create the worker.

	w, err := s.newWorker(c, s.newCloudSpecEnviron())
	c.Assert(err, tc.ErrorIsNil)

	s.ensureStartup(c)

	// Send a notification so that a credential change is picked up.

	select {
	case ch <- struct{}{}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out sending config change")
	}

	workertest.CleanKill(c, w)
}

func (s *trackerWorkerSuite) getConfig(c *tc.C, environ environs.Environ) TrackerConfig {
	return TrackerConfig{
		ModelService:      s.modelService,
		CloudService:      s.cloudService,
		ConfigService:     s.configService,
		CredentialService: s.credentialService,
		GetProviderForType: getProviderForType(
			IAASGetProvider(func(_ context.Context, _ environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
				c.Assert(invalidator, tc.Not(tc.IsNil))
				err := invalidator.InvalidateCredentials(c.Context(), "bad")
				if err != nil {
					return nil, err
				}
				return environ, nil
			}),
			CAASGetProvider(func(_ context.Context, _ environs.OpenParams, _ environs.CredentialInvalidator) (caas.Broker, error) {
				c.Fatal("unexpected call")
				return nil, nil
			}),
		),
		Logger: s.logger,
	}
}

func (s *trackerWorkerSuite) expectModel(c *tc.C) coremodel.UUID {
	id := modeltesting.GenModelUUID(c)

	s.modelService.EXPECT().Model(gomock.Any()).Return(coremodel.ModelInfo{
		UUID:            id,
		Name:            "model",
		Type:            coremodel.IAAS,
		Cloud:           "cloud",
		CredentialOwner: usertesting.GenNewName(c, "owner"),
		CredentialName:  "name",
	}, nil)

	return id
}

func (s *trackerWorkerSuite) newCloudSpec(c *tc.C) *config.Config {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, tc.ErrorIsNil)

	s.configService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)
	s.cloudService.EXPECT().Cloud(gomock.Any(), "cloud").Return(&cloud.Cloud{}, nil)
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), credential.Key{
		Cloud: "cloud",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "name",
	}).Return(cloud.Credential{}, nil)

	return cfg
}

func (s *trackerWorkerSuite) expectInvalidateCredential(c *tc.C) {
	s.credentialService.EXPECT().InvalidateCredential(gomock.Any(), credential.Key{
		Cloud: "cloud",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "name",
	}, "bad")
}

func (s *trackerWorkerSuite) expectCloudSpec(c *tc.C, cfg *config.Config) {
	s.environ.EXPECT().Config().Return(cfg)
}

func (s *trackerWorkerSuite) expectEnvironSetConfig(c *tc.C, cfg *config.Config) {
	s.configService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)
	s.environ.EXPECT().SetConfig(gomock.Any(), cfg)
}

func (s *trackerWorkerSuite) expectEnvironSetSpecUpdate(c *tc.C) {
	s.cloudService.EXPECT().Cloud(gomock.Any(), "cloud").Return(&cloud.Cloud{}, nil)
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), credential.Key{
		Cloud: "cloud",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "name",
	}).Return(cloud.Credential{
		Revoked: true,
	}, nil)
	s.cloudSpecSetter.EXPECT().SetCloudSpec(gomock.Any(), gomock.Any()).Return(nil)
}

func (s *trackerWorkerSuite) expectConfigWatcher(c *tc.C) chan []string {
	ch := make(chan []string)
	// Seed the initial event.
	go func() {
		select {
		case ch <- []string{}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out seeding initial event")
		}
	}()

	watcher := watchertest.NewMockStringsWatcher(ch)

	s.configService.EXPECT().Watch().Return(watcher, nil)

	return ch
}

func (s *trackerWorkerSuite) expectModelCloudCredentialWatcher(c *tc.C, uuid coremodel.UUID) chan struct{} {
	ch := make(chan struct{})
	// Seed the initial event.
	go func() {
		select {
		case ch <- struct{}{}:
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out seeding initial event")
		}
	}()

	watcher := watchertest.NewMockNotifyWatcher(ch)

	s.modelService.EXPECT().WatchModelCloudCredential(gomock.Any(), uuid).Return(watcher, nil)

	return ch
}

func (s *trackerWorkerSuite) newWorker(c *tc.C, environ environs.Environ) (*trackerWorker, error) {
	return newTrackerWorker(c.Context(), s.getConfig(c, environ), s.states)
}

func (s *trackerWorkerSuite) newCloudSpecEnviron() *cloudSpecEnviron {
	return &cloudSpecEnviron{
		Environ:         s.environ,
		CloudSpecSetter: s.cloudSpecSetter,
	}
}

type cloudSpecEnviron struct {
	environs.Environ
	environs.CloudSpecSetter
}
