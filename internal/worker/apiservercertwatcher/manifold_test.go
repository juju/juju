// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiservercertwatcher_test

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/apiservercertwatcher"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	manifold dependency.Manifold
	getter   dependency.Getter
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.getter = dt.StubGetter(map[string]any{})
	s.manifold = apiservercertwatcher.Manifold(apiservercertwatcher.ManifoldConfig{
		CACert:               coretesting.OtherCACert,
		CAPrivateKey:         coretesting.OtherCAKey,
		ControllerCert:       coretesting.ServerCert,
		ControllerPrivateKey: coretesting.ServerKey,
	})
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.manifold.Inputs, tc.HasLen, 0)
}

func (s *ManifoldSuite) TestValidate_EmptyCACert(c *tc.C) {
	cfg := apiservercertwatcher.ManifoldConfig{
		CAPrivateKey:         coretesting.OtherCAKey,
		ControllerCert:       coretesting.ServerCert,
		ControllerPrivateKey: coretesting.ServerKey,
	}
	c.Assert(cfg.Validate(), tc.ErrorMatches, `.*CACert.*not valid`)
}

func (s *ManifoldSuite) TestValidate_EmptyCAPrivateKey(c *tc.C) {
	cfg := apiservercertwatcher.ManifoldConfig{
		CACert:               coretesting.OtherCACert,
		ControllerCert:       coretesting.ServerCert,
		ControllerPrivateKey: coretesting.ServerKey,
	}
	c.Assert(cfg.Validate(), tc.ErrorMatches, `.*CAPrivateKey.*not valid`)
}

func (s *ManifoldSuite) TestValidate_EmptyControllerCert(c *tc.C) {
	cfg := apiservercertwatcher.ManifoldConfig{
		CACert:               coretesting.OtherCACert,
		CAPrivateKey:         coretesting.OtherCAKey,
		ControllerPrivateKey: coretesting.ServerKey,
	}
	c.Assert(cfg.Validate(), tc.ErrorMatches, `.*ControllerCert.*not valid`)
}

func (s *ManifoldSuite) TestValidate_EmptyControllerPrivateKey(c *tc.C) {
	cfg := apiservercertwatcher.ManifoldConfig{
		CACert:         coretesting.OtherCACert,
		CAPrivateKey:   coretesting.OtherCAKey,
		ControllerCert: coretesting.ServerCert,
	}
	c.Assert(cfg.Validate(), tc.ErrorMatches, `.*ControllerPrivateKey.*not valid`)
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) TestOutput(c *tc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	var authority pki.Authority
	err := s.manifold.Output(w, &authority)
	c.Assert(err, tc.ErrorIsNil)
}

// TestStart_NoAgentDependency verifies the manifold does not depend on agent.
func (s *ManifoldSuite) TestStart_NoAgentDependency(c *tc.C) {
	// The getter has no "agent" entry; start must succeed without it.
	getter := dt.StubGetter(map[string]any{})
	w, err := s.manifold.Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

// TestStart_CertFieldsPassedToWorker verifies that the explicit cert fields
// are passed to the worker constructor (via CertWatcherWorkerFn).
func (s *ManifoldSuite) TestStart_CertFieldsPassedToWorker(c *tc.C) {
	var gotCACert, gotCAKey, gotCert, gotKey string
	manifold := apiservercertwatcher.Manifold(apiservercertwatcher.ManifoldConfig{
		CACert:               coretesting.OtherCACert,
		CAPrivateKey:         coretesting.OtherCAKey,
		ControllerCert:       coretesting.ServerCert,
		ControllerPrivateKey: coretesting.ServerKey,
		CertWatcherWorkerFn: func(cfg apiservercertwatcher.ManifoldConfig) (apiservercertwatcher.AuthorityWorker, error) {
			gotCACert = cfg.CACert
			gotCAKey = cfg.CAPrivateKey
			gotCert = cfg.ControllerCert
			gotKey = cfg.ControllerPrivateKey
			return nil, dependency.ErrUninstall
		},
	})
	_, _ = manifold.Start(c.Context(), s.getter)
	c.Check(gotCACert, tc.Equals, coretesting.OtherCACert)
	c.Check(gotCAKey, tc.Equals, coretesting.OtherCAKey)
	c.Check(gotCert, tc.Equals, coretesting.ServerCert)
	c.Check(gotKey, tc.Equals, coretesting.ServerKey)
}

func (s *ManifoldSuite) startWorkerClean(c *tc.C) worker.Worker {
	w, err := s.manifold.Start(c.Context(), s.getter)
	c.Assert(err, tc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
}
