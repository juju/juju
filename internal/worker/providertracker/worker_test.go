// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/environs"
	cloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type workerSuite struct {
	baseSuite

	states chan string
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestWorkerStartup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)

	// Create the worker.

	w, err := s.newWorker(c, s.environ)
	c.Assert(err, jc.ErrorIsNil)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerStartupWithCloudSpec(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with the cloud spec setter and environ.

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)

	// Now we've got the cloud spec setter, we need to ensure we watch the
	// cloud and credentials.

	s.expectCloudWatcher(c)
	s.expectCredentialWatcher(c)

	// Create the worker.

	w, err := s.newWorker(c, s.newCloudSpecEnviron())
	c.Assert(err, jc.ErrorIsNil)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerModelConfigUpdatesEnviron(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	ch := s.expectConfigWatcher(c)
	s.expectEnvironSetConfig(c, cfg)

	// Create the worker.

	w, err := s.newWorker(c, s.environ)
	c.Assert(err, jc.ErrorIsNil)

	s.ensureStartup(c)

	// Dispatch a config change, ensure it's picked up via the environ.

	select {
	case ch <- []string{"foo"}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out sending config change")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerCloudUpdatesEnviron(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)

	// Now we've got the cloud spec setter, we need to ensure we watch the
	// cloud and credentials.

	ch := s.expectCloudWatcher(c)
	s.expectCredentialWatcher(c)

	// This will cause the cloud spec to be updated.
	s.expectEnvironSetSpecUpdate(c)

	// Create the worker.

	w, err := s.newWorker(c, s.newCloudSpecEnviron())
	c.Assert(err, jc.ErrorIsNil)

	s.ensureStartup(c)

	// Send a notification so that a cloud change is picked up.

	select {
	case ch <- struct{}{}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out sending config change")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerCredentialUpdatesEnviron(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)

	// Now we've got the cloud spec setter, we need to ensure we watch the
	// cloud and credentials.

	s.expectCloudWatcher(c)
	ch := s.expectCredentialWatcher(c)

	// This will cause the cloud spec to be updated.
	s.expectEnvironSetSpecUpdate(c)

	// Create the worker.

	w, err := s.newWorker(c, s.newCloudSpecEnviron())
	c.Assert(err, jc.ErrorIsNil)

	s.ensureStartup(c)

	// Send a notification so that a credential change is picked up.

	select {
	case ch <- struct{}{}:
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out sending config change")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) getConfig(environ environs.Environ) Config[environs.Environ] {
	return Config[environs.Environ]{
		ModelService:      s.modelService,
		CloudService:      s.cloudService,
		ConfigService:     s.configService,
		CredentialService: s.credentialService,
		GetProvider: func(ctx context.Context, pcg ProviderConfigGetter) (environs.Environ, cloudspec.CloudSpec, error) {
			return environ, cloudspec.CloudSpec{}, nil
		},
		Logger: s.logger,
	}
}

func (s *workerSuite) expectModel(c *gc.C) coremodel.UUID {
	id := modeltesting.GenModelUUID(c)

	s.modelService.EXPECT().Model(gomock.Any()).Return(coremodel.ReadOnlyModel{
		UUID:            id,
		Name:            "model",
		Type:            coremodel.IAAS,
		Cloud:           "cloud",
		CredentialOwner: "owner",
		CredentialName:  "name",
	}, nil)

	return id
}

func (s *workerSuite) newCloudSpec(c *gc.C) *config.Config {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)

	s.configService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)
	s.cloudService.EXPECT().Cloud(gomock.Any(), "cloud").Return(&cloud.Cloud{}, nil)
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), credential.Key{
		Cloud: "cloud",
		Owner: "owner",
		Name:  "name",
	}).Return(cloud.Credential{}, nil)

	return cfg
}

func (s *workerSuite) expectCloudSpec(c *gc.C, cfg *config.Config) {
	s.environ.EXPECT().Config().Return(cfg)
}

func (s *workerSuite) expectEnvironSetConfig(c *gc.C, cfg *config.Config) {
	s.configService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)
	s.environ.EXPECT().SetConfig(cfg)
}

func (s *workerSuite) expectEnvironSetSpecUpdate(c *gc.C) {
	s.cloudService.EXPECT().Cloud(gomock.Any(), "cloud").Return(&cloud.Cloud{}, nil)
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), credential.Key{
		Cloud: "cloud",
		Owner: "owner",
		Name:  "name",
	}).Return(cloud.Credential{
		Revoked: true,
	}, nil)
	s.cloudSpecSetter.EXPECT().SetCloudSpec(gomock.Any(), gomock.Any()).Return(nil)
}

func (s *workerSuite) expectEnvironSetSpecNoUpdate(c *gc.C) {
	s.cloudService.EXPECT().Cloud(gomock.Any(), "cloud").Return(&cloud.Cloud{}, nil)
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), credential.Key{
		Cloud: "cloud",
		Owner: "owner",
		Name:  "name",
	}).Return(cloud.Credential{}, nil)
}

func (s *workerSuite) expectConfigWatcher(c *gc.C) chan []string {
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

func (s *workerSuite) expectCloudWatcher(c *gc.C) chan struct{} {
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

	s.cloudService.EXPECT().WatchCloud(gomock.Any(), "cloud").Return(watcher, nil)

	return ch
}

func (s *workerSuite) expectCredentialWatcher(c *gc.C) chan struct{} {
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

	s.credentialService.EXPECT().WatchCredential(gomock.Any(), credential.Key{
		Cloud: "cloud",
		Owner: "owner",
		Name:  "name",
	}).Return(watcher, nil)

	return ch
}

func (s *workerSuite) newWorker(c *gc.C, environ environs.Environ) (*trackerWorker[environs.Environ], error) {
	return newWorker(context.Background(), s.getConfig(environ), s.states)
}

func (s *workerSuite) newCloudSpecEnviron() *cloudSpecEnviron {
	return &cloudSpecEnviron{
		Environ:         s.environ,
		CloudSpecSetter: s.cloudSpecSetter,
	}
}

func (s *workerSuite) setupMocks(c *gc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	return s.baseSuite.setupMocks(c)
}

func (s *workerSuite) ensureStartup(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

type cloudSpecEnviron struct {
	environs.Environ
	environs.CloudSpecSetter
}
