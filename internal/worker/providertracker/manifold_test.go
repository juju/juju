// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/environs"
	cloudspec "github.com/juju/juju/environs/cloudspec"
	caas "github.com/juju/juju/internal/provider/caas"
	"github.com/juju/juju/internal/servicefactory"
	storage "github.com/juju/juju/internal/storage"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.ProviderServiceFactoriesName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.GetProvider = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewTrackerWorker = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.GetProviderServiceFactoryGetter = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig[environs.Environ] {
	return ManifoldConfig[environs.Environ]{
		ProviderServiceFactoriesName: "provider-service-factory",
		Logger:                       s.logger,
		Clock:                        clock.WallClock,
		NewWorker: func(cfg Config[environs.Environ]) (worker.Worker, error) {
			return newStubWorker(), nil
		},
		NewTrackerWorker: func(ctx context.Context, cfg TrackerConfig[environs.Environ]) (worker.Worker, error) {
			return newStubWorker(), nil
		},
		NewProvider: func(ctx context.Context, args environs.OpenParams) (environs.Environ, error) {
			return s.environ, nil
		},
		GetProvider: func(ctx context.Context, pcg ProviderConfigGetter) (environs.Environ, cloudspec.CloudSpec, error) {
			return s.environ, cloudspec.CloudSpec{}, nil
		},
		GetProviderServiceFactoryGetter: func(getter dependency.Getter, name string) (ServiceFactoryGetter, error) {
			return s.serviceFactoryGetter, nil
		},
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"provider-service-factory": &stubProviderServiceFactory{},
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"provider-service-factory"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(MultiTrackerManifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := MultiTrackerManifold(s.getConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestIAASManifoldOutput(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectServiceFactory("hunter2")

	w, err := newWorker(Config[environs.Environ]{
		TrackerType:          SingularType("hunter2"),
		ServiceFactoryGetter: s.serviceFactoryGetter,
		GetProvider: func(ctx context.Context, pcg ProviderConfigGetter) (environs.Environ, cloudspec.CloudSpec, error) {
			return s.environ, cloudspec.CloudSpec{}, nil
		},
		NewTrackerWorker: func(ctx context.Context, cfg TrackerConfig[environs.Environ]) (worker.Worker, error) {
			w := &trackerWorker[environs.Environ]{
				provider: s.environ,
			}
			err := catacomb.Invoke(catacomb.Plan{
				Site: &w.catacomb,
				Work: func() error {
					<-w.catacomb.Dying()
					return w.catacomb.ErrDying()
				},
			})
			return w, err
		},
		Logger: s.logger,
		Clock:  clock.WallClock,
	}, s.states)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	var environ environs.Environ
	err = manifoldOutput[environs.Environ](w, &environ)
	c.Check(err, jc.ErrorIsNil)

	var destroyer environs.CloudDestroyer
	err = manifoldOutput[environs.Environ](w, &destroyer)
	c.Check(err, jc.ErrorIsNil)

	var registry storage.ProviderRegistry
	err = manifoldOutput[environs.Environ](w, &registry)
	c.Check(err, jc.ErrorIsNil)

	var bob string
	err = manifoldOutput[environs.Environ](w, &bob)
	c.Check(err, gc.ErrorMatches, `expected \*environs.Environ, \*storage.ProviderRegistry, or \*environs.CloudDestroyer, got \*string`)
}

func (s *manifoldSuite) TestCAASManifoldOutput(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectServiceFactory("hunter2")

	w, err := newWorker(Config[caas.Broker]{
		TrackerType:          SingularType("hunter2"),
		ServiceFactoryGetter: s.serviceFactoryGetter,
		GetProvider: func(ctx context.Context, pcg ProviderConfigGetter) (caas.Broker, cloudspec.CloudSpec, error) {
			return s.broker, cloudspec.CloudSpec{}, nil
		},
		NewTrackerWorker: func(ctx context.Context, cfg TrackerConfig[caas.Broker]) (worker.Worker, error) {
			w := &trackerWorker[caas.Broker]{
				provider: s.broker,
			}
			err := catacomb.Invoke(catacomb.Plan{
				Site: &w.catacomb,
				Work: func() error {
					<-w.catacomb.Dying()
					return w.catacomb.ErrDying()
				},
			})
			return w, err
		},
		Logger: s.logger,
		Clock:  clock.WallClock,
	}, s.states)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.ensureStartup(c)

	var broker caas.Broker
	err = manifoldOutput[caas.Broker](w, &broker)
	c.Check(err, jc.ErrorIsNil)

	var destroyer environs.CloudDestroyer
	err = manifoldOutput[caas.Broker](w, &destroyer)
	c.Check(err, jc.ErrorIsNil)

	var registry storage.ProviderRegistry
	err = manifoldOutput[caas.Broker](w, &registry)
	c.Check(err, jc.ErrorIsNil)

	var bob string
	err = manifoldOutput[caas.Broker](w, &bob)
	c.Check(err, gc.ErrorMatches, `expected \*caas.Broker, \*storage.ProviderRegistry or \*environs.CloudDestroyer, got \*string`)
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

type stubProviderServiceFactory struct {
	servicefactory.ProviderServiceFactory
}
