// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretspruner_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/secretspruner"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite
	config secretspruner.ManifoldConfig
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig(c)
}

func (s *manifoldSuite) validConfig(c *tc.C) secretspruner.ManifoldConfig {
	return secretspruner.ManifoldConfig{
		DomainServicesName: "domain-services",
		Logger:             loggertesting.WrapCheckLog(c),
		NewWorker: func(config secretspruner.Config) (worker.Worker, error) {
			return nil, nil
		},
	}
}

func (s *manifoldSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *manifoldSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *manifoldSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *manifoldSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *manifoldSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	m := secretspruner.Manifold(s.config)
	c.Check(m.Inputs, tc.DeepEquals, []string{"domain-services"})
}

func (s *manifoldSuite) TestStartMissingDomainServices(c *tc.C) {
	getter := dt.StubGetter(map[string]any{
		"domain-services": dependency.ErrMissing,
	})
	m := secretspruner.Manifold(s.config)
	w, err := m.Start(c.Context(), getter)
	c.Check(w, tc.IsNil)
	c.Check(err, tc.ErrorIs, dependency.ErrMissing)
}
