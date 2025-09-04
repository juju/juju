// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operationpruner

import (
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	loggertesting "github.com/juju/juju/internal/logger/testing"
)

const domainServicesName = "domain-services"

type manifoldSuite struct{}

func TestManifoldSuite(t *testing.T) { tc.Run(t, &manifoldSuite{}) }

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	cfg := s.newConfig(c)

	c.Check(cfg.Validate(), tc.ErrorIsNil)

	bad := cfg
	bad.DomainServicesName = ""
	c.Check(bad.Validate(), tc.ErrorIs, errors.NotValid)

	bad = cfg
	bad.Clock = nil
	c.Check(bad.Validate(), tc.ErrorIs, errors.NotValid)

	bad = cfg
	bad.Logger = nil
	c.Check(bad.Validate(), tc.ErrorIs, errors.NotValid)

	bad = cfg
	bad.PruneInterval = 0
	c.Check(bad.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestStartMissingDomainServices(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		domainServicesName: dependency.ErrMissing,
	})

	w, err := s.newManifold(c).Start(c.Context(), getter)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorIs, dependency.ErrMissing)
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.newManifold(c).Inputs, tc.DeepEquals, []string{
		domainServicesName,
	})
}

func (s *manifoldSuite) newManifold(c *tc.C) dependency.Manifold {
	return Manifold(s.newConfig(c))
}

func (s *manifoldSuite) newConfig(c *tc.C) ManifoldConfig {
	cfg := ManifoldConfig{
		DomainServicesName: domainServicesName,
		Clock:              testclock.NewClock(time.Now()),
		Logger:             loggertesting.WrapCheckLog(c),
		PruneInterval:      time.Second,
	}
	return cfg
}
