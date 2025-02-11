// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	context "context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	caas "github.com/juju/juju/caas"
	coremodel "github.com/juju/juju/core/model"
	environs "github.com/juju/juju/environs"
	config "github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type nonTrackerWorkerSuite struct {
	baseSuite
}

var _ = gc.Suite(&nonTrackerWorkerSuite{})

func (s *nonTrackerWorkerSuite) TestWorkerStartup(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Create the worker.
	w, err := s.newWorker(c, s.environ)
	c.Assert(err, jc.ErrorIsNil)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)
}

func (s *nonTrackerWorkerSuite) TestWorkerProvider(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Create the worker.
	w, err := s.newWorker(c, s.environ)
	c.Assert(err, jc.ErrorIsNil)

	s.ensureStartup(c)

	p, err := w.Provider()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(p, gc.Equals, s.environ)

	workertest.CleanKill(c, w)
}

func (s *nonTrackerWorkerSuite) newWorker(c *gc.C, environ environs.Environ) (*nonTrackedWorker, error) {
	return newNonTrackedWorker(context.Background(), s.getConfig(c, environ), s.states)
}

func (s *nonTrackerWorkerSuite) getConfig(c *gc.C, environ environs.Environ) NonTrackedConfig {
	cfg, err := config.New(config.NoDefaults, testing.FakeConfig())
	c.Assert(err, jc.ErrorIsNil)

	return NonTrackedConfig{
		ModelType:      coremodel.IAAS,
		ModelConfig:    cfg,
		CloudSpec:      testing.FakeCloudSpec(),
		ControllerUUID: uuid.MustNewUUID(),
		GetProviderForType: getProviderForType(
			IAASGetProvider(func(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
				return environ, nil
			}),
			CAASGetProvider(func(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (caas.Broker, error) {
				c.Fatal("unexpected call")
				return nil, nil
			}),
		),
		Logger: s.logger,
	}
}
