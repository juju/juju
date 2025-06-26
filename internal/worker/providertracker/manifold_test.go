// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/storage"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.ProviderServiceFactoriesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.GetIAASProvider = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.GetCAASProvider = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewTrackerWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewEphemeralProvider = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.GetProviderServicesGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		ProviderServiceFactoriesName: "provider-services",
		Logger:                       s.logger,
		Clock:                        clock.WallClock,
		NewWorker: func(cfg Config) (worker.Worker, error) {
			return newStubWorker(), nil
		},
		NewTrackerWorker: func(ctx context.Context, cfg TrackerConfig) (worker.Worker, error) {
			return newStubWorker(), nil
		},
		NewEphemeralProvider: func(ctx context.Context, cfg EphemeralConfig) (Provider, error) {
			return nil, nil
		},
		GetIAASProvider: func(ctx context.Context, ecg environs.EnvironConfigGetter, invalidator environs.CredentialInvalidator) (Provider, cloudspec.CloudSpec, error) {
			return s.environ, cloudspec.CloudSpec{}, nil
		},
		GetCAASProvider: func(ctx context.Context, ecg environs.EnvironConfigGetter, invalidator environs.CredentialInvalidator) (Provider, cloudspec.CloudSpec, error) {
			return s.broker, cloudspec.CloudSpec{}, nil
		},
		GetProviderServicesGetter: func(getter dependency.Getter, name string) (DomainServicesGetter, error) {
			return s.domainServicesGetter, nil
		},
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"provider-services": &stubProviderServices{},
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"provider-services"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(MultiTrackerManifold(s.getConfig()).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := MultiTrackerManifold(s.getConfig()).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestIAASManifoldOutput(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDomainServices("hunter2")

	w, err := newWorker(Config{
		TrackerType:          SingularType("hunter2"),
		DomainServicesGetter: s.domainServicesGetter,
		GetIAASProvider: func(ctx context.Context, ecg environs.EnvironConfigGetter, invalidator environs.CredentialInvalidator) (Provider, cloudspec.CloudSpec, error) {
			return s.environ, cloudspec.CloudSpec{}, nil
		},
		GetCAASProvider: func(ctx context.Context, ecg environs.EnvironConfigGetter, invalidator environs.CredentialInvalidator) (Provider, cloudspec.CloudSpec, error) {
			return s.broker, cloudspec.CloudSpec{}, nil
		},
		NewTrackerWorker: func(ctx context.Context, cfg TrackerConfig) (worker.Worker, error) {
			w := &trackerWorker{
				provider: s.environ,
			}
			err := catacomb.Invoke(catacomb.Plan{
				Name: "tracker-worker",
				Site: &w.catacomb,
				Work: func() error {
					<-w.catacomb.Dying()
					return w.catacomb.ErrDying()
				},
			})
			return w, err
		},
		NewEphemeralProvider: func(ctx context.Context, cfg EphemeralConfig) (Provider, error) {
			c.Fail()
			return nil, nil
		},
		Logger: s.logger,
		Clock:  clock.WallClock,
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	var environ environs.Environ
	err = manifoldOutput(w, &environ)
	c.Check(err, tc.ErrorIsNil)

	var destroyer environs.CloudDestroyer
	err = manifoldOutput(w, &destroyer)
	c.Check(err, tc.ErrorIsNil)

	var registry storage.ProviderRegistry
	err = manifoldOutput(w, &registry)
	c.Check(err, tc.ErrorIsNil)

	var bob string
	err = manifoldOutput(w, &bob)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestCAASManifoldOutput(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectDomainServices("hunter2")

	w, err := newWorker(Config{
		TrackerType:          SingularType("hunter2"),
		DomainServicesGetter: s.domainServicesGetter,
		GetIAASProvider: func(ctx context.Context, ecg environs.EnvironConfigGetter, invalidator environs.CredentialInvalidator) (Provider, cloudspec.CloudSpec, error) {
			return s.environ, cloudspec.CloudSpec{}, nil
		},
		GetCAASProvider: func(ctx context.Context, ecg environs.EnvironConfigGetter, invalidator environs.CredentialInvalidator) (Provider, cloudspec.CloudSpec, error) {
			return s.broker, cloudspec.CloudSpec{}, nil
		},
		NewTrackerWorker: func(ctx context.Context, cfg TrackerConfig) (worker.Worker, error) {
			w := &trackerWorker{
				provider: s.broker,
			}
			err := catacomb.Invoke(catacomb.Plan{
				Name: "tracker-worker",
				Site: &w.catacomb,
				Work: func() error {
					<-w.catacomb.Dying()
					return w.catacomb.ErrDying()
				},
			})
			return w, err
		},
		NewEphemeralProvider: func(ctx context.Context, cfg EphemeralConfig) (Provider, error) {
			c.Fail()
			return nil, nil
		},
		Logger: s.logger,
		Clock:  clock.WallClock,
	}, s.states)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	var broker caas.Broker
	err = manifoldOutput(w, &broker)
	c.Check(err, tc.ErrorIsNil)

	var destroyer environs.CloudDestroyer
	err = manifoldOutput(w, &destroyer)
	c.Check(err, tc.ErrorIsNil)

	var registry storage.ProviderRegistry
	err = manifoldOutput(w, &registry)
	c.Check(err, tc.ErrorIsNil)

	var bob string
	err = manifoldOutput(w, &bob)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

type stubWorker struct {
	tomb tomb.Tomb
}

func newStubWorker() *stubWorker {
	w := &stubWorker{}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
	})
	return w
}

func (w *stubWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *stubWorker) Wait() error {
	return w.tomb.Wait()
}

type stubProviderServices struct {
	services.ProviderServices
}
