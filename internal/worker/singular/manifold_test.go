// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"context"
	"testing"
	"time"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	manager *MockManager

	modelUUID string
}

func TestSingularManifoldSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &ManifoldSuite{})
	})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.modelUUID = uuid.MustNewUUID().String()
}

func (s *ManifoldSuite) TestValidate(c *tc.C) {
	config := s.newConfig()
	c.Assert(config.Validate(), tc.ErrorIsNil)

	config = s.newConfig()
	config.ModelUUID = ""
	c.Assert(config.Validate(), tc.ErrorIs, errors.NotValid)

	config = s.newConfig()
	config.LeaseManagerName = ""
	c.Assert(config.Validate(), tc.ErrorIs, errors.NotValid)

	config = s.newConfig()
	config.Clock = nil
	c.Assert(config.Validate(), tc.ErrorIs, errors.NotValid)

	config = s.newConfig()
	config.NewWorker = nil
	c.Assert(config.Validate(), tc.ErrorIs, errors.NotValid)

	config = s.newConfig()
	config.Claimant = names.NewUserTag("bob")
	c.Assert(config.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) newConfig() ManifoldConfig {
	return ManifoldConfig{
		ModelUUID:        s.modelUUID,
		LeaseManagerName: "lease-manager",
		Clock:            clock.WallClock,
		Duration:         time.Minute,
		Entity:           names.NewModelTag("model-123"),
		Claimant:         names.NewMachineTag("123"),
		NewWorker: func(ctx context.Context, config FlagConfig) (worker.Worker, error) {
			return newStubWorker(), nil
		},
	}
}

func (s *ManifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"lease-manager": s.manager,
	}
	return dependencytesting.StubGetter(resources)
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.newConfig()).Inputs, tc.SameContents, []string{"lease-manager"})
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.newConfig()).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) TestWorkerBounceOnStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := s.newConfig()
	config.NewWorker = func(context.Context, FlagConfig) (worker.Worker, error) {
		return nil, ErrRefresh
	}

	_, err := Manifold(config).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIs, dependency.ErrBounce)
}

func (s *ManifoldSuite) TestStartModelUUIDPassedToFlagConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var gotUUID string
	config := s.newConfig()
	config.NewWorker = func(_ context.Context, cfg FlagConfig) (worker.Worker, error) {
		gotUUID = string(cfg.ModelUUID)
		return newStubWorker(), nil
	}

	w, err := Manifold(config).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
	c.Check(gotUUID, tc.Equals, s.modelUUID)
}

func (s *ManifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.manager = NewMockManager(ctrl)

	return ctrl
}

type stubWorker struct {
	tomb.Tomb
}

func newStubWorker() *stubWorker {
	w := &stubWorker{}
	w.Tomb.Go(func() error {
		<-w.Tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

func (w *stubWorker) Kill() {
	w.Tomb.Kill(nil)
}

func (w *stubWorker) Wait() error {
	return w.Tomb.Wait()
}

func TestNotPrimaryControllerFlagManifold(c *testing.T) {
	testhelpers.PrintGoroutineLeaks(c, func(c *testing.T) {
		tc.Run(c, &notPrimaryControllerFlagManifoldSuite{})
	})
}

type notPrimaryControllerFlagManifoldSuite struct{}

func (*notPrimaryControllerFlagManifoldSuite) TestOutputsInverseFlag(c *tc.C) {
	for _, test := range []struct {
		name     string
		input    bool
		expected bool
	}{
		{name: "source true", input: true, expected: false},
		{name: "source false", input: false, expected: true},
	} {
		source := engine.NewStaticFlagWorker(test.input)
		manifold := NotPrimaryControllerFlagManifold("source")
		worker, err := manifold.Start(c.Context(), dependencytesting.StubGetter(map[string]any{
			"source": source,
		}))
		c.Assert(err, tc.ErrorIsNil)

		var flag engine.Flag
		c.Assert(manifold.Output(worker, &flag), tc.ErrorIsNil)
		c.Check(flag.Check(), tc.Equals, test.expected, tc.Commentf("%s", test.name))

		workertest.CleanKill(c, worker)
		workertest.CleanKill(c, source)
	}
}
