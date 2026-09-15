// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"
	stdtesting "testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/testing"
)

type trackerWorkerSuite struct {
	baseSuite
}

func TestTrackerWorkerSuite(t *stdtesting.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *stdtesting.T) {
		tc.Run(t, &trackerWorkerSuite{})
	})
}

func (s *trackerWorkerSuite) TestWorkerStartup(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)
	s.expectModelWatcher(c)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// Create the worker.

	w, err := s.newWorker(c, s.environ)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)
}

// TestChangeAfterReadyModelDeath verifies that a model death notification
// arriving after the watcher readiness barrier causes the worker to stop.
// This exercises the race between watcher subscription and the initial
// state query.
func (s *trackerWorkerSuite) TestChangeAfterReadyModelDeath(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)
	ch := s.expectModelWatcher(c)

	s.modelService.EXPECT().Model(gomock.Any()).Return(coremodel.ModelInfo{
		Life: corelife.Dead,
	}, nil)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// Create the worker.

	w, err := s.newWorker(c, s.environ)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out sending model")
	}

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIsNil)
}

// TestChangeAfterReadyModelNotFound verifies that a model removal
// notification arriving after the watcher readiness barrier causes the
// worker to stop. This exercises the race between watcher subscription and
// the initial state query.
func (s *trackerWorkerSuite) TestChangeAfterReadyModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)
	ch := s.expectModelWatcher(c)

	s.modelService.EXPECT().Model(gomock.Any()).Return(coremodel.ModelInfo{}, modelerrors.NotFound)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// Create the worker.

	w, err := s.newWorker(c, s.environ)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out sending model")
	}

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *trackerWorkerSuite) TestWorkerStartupWithCloudSpec(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with the cloud spec setter and environ.

	uuid := s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)
	s.expectModelWatcher(c)

	// Now we've got the cloud spec setter, we need to ensure we watch the
	// cloud and credentials.

	s.expectModelCloudCredentialWatcher(c, uuid)

	// The startup sync reads the cloud spec once after subscribing the
	// credential watcher. Since no credential change occurred, the spec
	// matches and no SetCloudSpec is called.
	s.expectCloudSpecRead(c)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// Create the worker.

	w, err := s.newWorker(c, s.newCloudSpecEnviron())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)
}

// TestChangeAfterReadyUpdatesConfig verifies that a model config change
// arriving after the watcher readiness barrier is caught and applied to the
// provider. This exercises the race between watcher subscription and the
// initial state query.
func (s *trackerWorkerSuite) TestChangeAfterReadyUpdatesConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	ch := s.expectConfigWatcher(c)
	s.expectEnvironSetConfig(c, cfg)
	s.expectModelWatcher(c)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// Create the worker.

	w, err := s.newWorker(c, s.environ)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Dispatch a config change, ensure it's picked up via the environ.

	select {
	case ch <- []string{"foo"}:
	case <-c.Context().Done():
		c.Fatalf("timed out sending config change")
	}

	workertest.CleanKill(c, w)
}

// TestChangeAfterReadyUpdatesCloudSpec verifies that a cloud credential
// change arriving after the watcher readiness barrier triggers a cloud spec
// update on the provider. This exercises the race between watcher
// subscription and the initial state query.
func (s *trackerWorkerSuite) TestChangeAfterReadyUpdatesCloudSpec(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	uuid := s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)
	s.expectModelWatcher(c)

	// Now we've got the cloud spec setter, we need to ensure we watch the
	// cloud and credentials.

	ch := s.expectModelCloudCredentialWatcher(c, uuid)

	// The startup sync reads the cloud spec once after subscribing the
	// credential watcher. Since no credential change occurred, the spec
	// matches and no SetCloudSpec is called.
	s.expectCloudSpecRead(c)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// This will cause the cloud spec to be updated.
	s.expectEnvironSetSpecUpdate(c)

	// Create the worker.

	w, err := s.newWorker(c, s.newCloudSpecEnviron())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Send a notification so that a cloud change is picked up.

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out sending config change")
	}

	workertest.CleanKill(c, w)
}

// TestChangeAfterReadyUpdatesCredentials verifies that a credential
// notification arriving after the watcher readiness barrier triggers a cloud
// spec update on the provider. This exercises the race between watcher
// subscription and the initial state query.
func (s *trackerWorkerSuite) TestChangeAfterReadyUpdatesCredentials(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Ensure we can startup with a normal environ.

	uuid := s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)
	s.expectModelWatcher(c)

	// Now we've got the cloud spec setter, we need to ensure we watch the
	// cloud and credentials.

	ch := s.expectModelCloudCredentialWatcher(c, uuid)

	// The startup sync reads the cloud spec once after subscribing the
	// credential watcher. Since no credential change occurred, the spec
	// matches and no SetCloudSpec is called.
	s.expectCloudSpecRead(c)

	// We call InvalidateCredential in the mock setup
	// to ensure it's wired up.
	s.expectInvalidateCredential(c)

	// This will cause the cloud spec to be updated.
	s.expectEnvironSetSpecUpdate(c)

	// Create the worker.

	w, err := s.newWorker(c, s.newCloudSpecEnviron())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Send a notification so that a credential change is picked up.

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out sending config change")
	}

	workertest.CleanKill(c, w)
}

// TestChangeDuringStartupConfigChange verifies that a model config change
// arriving during the subscription window (before the initial event is
// consumed) is not lost. The change is pre-seeded on a buffered channel
// alongside the initial event. The worker consumes the initial event as a
// readiness barrier, builds the provider, then processes the change event in
// the main loop. This exercises the race between watcher construction,
// subscription, and the initial query (rule 6).
func (s *trackerWorkerSuite) TestChangeDuringStartupConfigChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)

	// Pre-seed the config watcher channel with the initial event AND a
	// config change event on a buffered channel. The initial event is
	// consumed by addStringsWatcher (readiness barrier); the change event
	// is processed in the main loop after the provider is built.
	ch := make(chan []string, 2)
	ch <- []string{}
	ch <- []string{"foo"}
	configWatcher := watchertest.NewMockStringsWatcher(ch)
	s.configService.EXPECT().Watch(gomock.Any()).Return(configWatcher, nil)

	s.expectModelWatcher(c)

	// Expect the config change to be processed: ModelConfig is re-read
	// and SetConfig is called on the environ. Use DoAndReturn on
	// SetConfig to signal when the change has been processed, avoiding
	// a race with CleanKill.
	s.configService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)
	setConfigDone := make(chan struct{})
	s.environ.EXPECT().SetConfig(gomock.Any(), cfg).DoAndReturn(func(context.Context, *config.Config) error {
		close(setConfigDone)
		return nil
	})

	s.expectInvalidateCredential(c)

	w, err := s.newWorker(c, s.environ)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	// Wait for the config change to be processed by the loop.
	select {
	case <-setConfigDone:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for config change to be processed")
	}

	workertest.CleanKill(c, w)
}

// TestChangeDuringStartupModelDeath verifies that a model death notification
// arriving during the subscription window is not lost. The death event is
// pre-seeded on a buffered channel alongside the initial event. This
// exercises the race between watcher construction, subscription, and the
// initial query (rule 6).
func (s *trackerWorkerSuite) TestChangeDuringStartupModelDeath(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)

	// Pre-seed the model watcher channel with the initial event AND a
	// model change event on a buffered channel. The initial event is
	// consumed by addNotifyWatcher (readiness barrier); the change event
	// is processed in the main loop after the provider is built.
	ch := make(chan struct{}, 2)
	ch <- struct{}{}
	ch <- struct{}{}
	modelWatcher := watchertest.NewMockNotifyWatcher(ch)
	s.modelService.EXPECT().WatchModel(gomock.Any()).Return(modelWatcher, nil)

	// When the change event is processed, the model is read again and
	// found to be dead, causing the worker to stop.
	s.modelService.EXPECT().Model(gomock.Any()).Return(coremodel.ModelInfo{
		Life: corelife.Dead,
	}, nil)

	s.expectInvalidateCredential(c)

	w, err := s.newWorker(c, s.environ)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	err = workertest.CheckKilled(c, w)
	c.Assert(err, tc.ErrorIsNil)
}

// TestChangeDuringStartupCredentialChange verifies that a credential change
// occurring between building the provider and subscribing the credential
// watcher is caught by the startup sync. The credential service returns a
// revoked credential for the startup sync (different from the initial build),
// causing updateCloudSpec to call SetCloudSpec during startup. This exercises
// the race between the initial state query and the credential watcher
// subscription (rule 6).
func (s *trackerWorkerSuite) TestChangeDuringStartupCredentialChange(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uuid := s.expectModel(c)
	cfg := s.newCloudSpec(c)
	s.expectCloudSpec(c, cfg)
	s.expectConfigWatcher(c)
	s.expectModelWatcher(c)

	s.expectModelCloudCredentialWatcher(c, uuid)

	// The startup sync reads the cloud spec and finds the credential has
	// been revoked since the initial build. This causes SetCloudSpec to
	// be called during startup, catching the credential change that
	// occurred between the provider build and the credential watcher
	// subscription.
	s.cloudService.EXPECT().Cloud(gomock.Any(), "cloud").Return(&cloud.Cloud{}, nil)
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), credential.Key{
		Cloud: "cloud",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "name",
	}).Return(cloud.Credential{
		Revoked: true,
	}, nil)
	s.cloudSpecSetter.EXPECT().SetCloudSpec(gomock.Any(), gomock.Any()).Return(nil)

	s.expectInvalidateCredential(c)

	w, err := s.newWorker(c, s.newCloudSpecEnviron())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

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
	id := tc.Must0(c, coremodel.NewUUID)

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

// expectCloudSpecRead sets up expectations for one CloudSpec read (Cloud +
// CloudCredential) returning the initial non-revoked credential. This is
// used for the startup sync in tests where the provider implements
// CloudSpecSetter.
func (s *trackerWorkerSuite) expectCloudSpecRead(c *tc.C) {
	s.cloudService.EXPECT().Cloud(gomock.Any(), "cloud").Return(&cloud.Cloud{}, nil)
	s.credentialService.EXPECT().CloudCredential(gomock.Any(), credential.Key{
		Cloud: "cloud",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "name",
	}).Return(cloud.Credential{}, nil)
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
	go func() {
		ch <- []string{}
	}()

	watcher := watchertest.NewMockStringsWatcher(ch)

	s.configService.EXPECT().Watch(gomock.Any()).Return(watcher, nil)

	return ch
}

func (s *trackerWorkerSuite) expectModelWatcher(c *tc.C) chan struct{} {
	ch := make(chan struct{})
	go func() {
		ch <- struct{}{}
	}()

	watcher := watchertest.NewMockNotifyWatcher(ch)

	s.modelService.EXPECT().WatchModel(gomock.Any()).Return(watcher, nil)

	return ch
}

func (s *trackerWorkerSuite) expectModelCloudCredentialWatcher(c *tc.C, uuid coremodel.UUID) chan struct{} {
	ch := make(chan struct{})
	go func() {
		ch <- struct{}{}
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
