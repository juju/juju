// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authenticationworker

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5"

	"github.com/juju/juju/internal/testhelpers"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestManifoldHasOutput(c *tc.C) {
	manifold := Manifold(ManifoldConfig{
		AgentName:     "agent",
		APICallerName: "api-caller",
	})
	c.Check(manifold.Output, tc.NotNil)
	c.Check(manifold.Inputs, tc.SameContents, []string{"agent", "api-caller"})
}

func (s *manifoldSuite) TestOutputExtractsEphemeralKeysUpdater(c *tc.C) {
	in := &AuthWorker{Worker: &stubWorker{}}

	var updater EphemeralKeysUpdater
	err := output(in, &updater)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(updater, tc.Equals, in)
}

func (s *manifoldSuite) TestOutputWrongInputType(c *tc.C) {
	var updater EphemeralKeysUpdater
	err := output(&stubWorker{}, &updater)
	c.Check(err, tc.ErrorMatches, `expected \*AuthWorker, got .*`)
}

func (s *manifoldSuite) TestOutputWrongOutputType(c *tc.C) {
	in := &AuthWorker{Worker: &stubWorker{}}

	var wrong worker.Worker
	err := output(in, &wrong)
	c.Check(err, tc.ErrorMatches, `expected \*EphemeralKeysUpdater, got .*`)
}

type stubWorker struct{}

func (*stubWorker) Kill()       {}
func (*stubWorker) Wait() error { return nil }
