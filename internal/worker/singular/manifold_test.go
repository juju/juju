// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/uuid"
)

type ManifoldSuite struct {
	testing.IsolationSuite

	agent       *MockAgent
	agentConfig *MockConfig
	manager     *MockManager

	modelTag names.ModelTag
}

var _ = tc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.modelTag = names.NewModelTag(uuid.MustNewUUID().String())
}

func (s *ManifoldSuite) TestValidate(c *tc.C) {
	config := s.newConfig()
	c.Assert(config.Validate(), tc.ErrorIsNil)

	config = s.newConfig()
	config.AgentName = ""
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
		AgentName:        "agent",
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
		"agent":         s.agent,
		"lease-manager": s.manager,
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"agent", "lease-manager"}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.newConfig()).Inputs, tc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAgentConfig(c)

	w, err := Manifold(s.newConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) TestWorkerBounceOnStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAgentConfig(c)

	config := ManifoldConfig{
		AgentName:        "agent",
		LeaseManagerName: "lease-manager",
		Clock:            clock.WallClock,
		Duration:         time.Minute,
		Entity:           names.NewModelTag("model-123"),
		Claimant:         names.NewMachineTag("123"),
		NewWorker: func(ctx context.Context, config FlagConfig) (worker.Worker, error) {
			return nil, ErrRefresh
		},
	}

	_, err := Manifold(config).Start(context.Background(), s.newGetter())
	c.Assert(err, tc.ErrorIs, dependency.ErrBounce)
}

func (s *ManifoldSuite) expectAgentConfig(c *tc.C) {
	s.agentConfig.EXPECT().Model().Return(s.modelTag)
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig)
}

func (s *ManifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
	s.agentConfig = NewMockConfig(ctrl)
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
