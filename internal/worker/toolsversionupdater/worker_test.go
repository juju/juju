// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionupdater

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type workerSuite struct {
	testhelpers.IsolationSuite

	agentBinaryService *MockAgentBinaryService
	modelAgentService  *MockModelAgentService
}

func TestWorkerSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestNewWorkerGetsMissingArch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	targetVersion := semversion.MustParse("4.0.1")

	done := make(chan struct{})
	s.modelAgentService.EXPECT().GetMissingAgentTargetVersions(gomock.Any()).Return(targetVersion, []arch.Arch{arch.S390X}, nil)
	s.agentBinaryService.EXPECT().RetrieveExternalAgentBinary(gomock.Any(), agentbinary.Version{
		Number: targetVersion,
		Arch:   arch.S390X,
	}).DoAndReturn(func(ctx context.Context, v agentbinary.Version) (*service.ComputedHashes, error) {
		close(done)
		return nil, nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timeout waiting for agent binary retrieval")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNewWorkerGetsMultipleMissingArch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	targetVersion := semversion.MustParse("4.0.1")

	done := make(chan struct{})
	s.modelAgentService.EXPECT().GetMissingAgentTargetVersions(gomock.Any()).Return(targetVersion, []arch.Arch{arch.S390X, arch.PPC64EL}, nil)
	s.agentBinaryService.EXPECT().RetrieveExternalAgentBinary(gomock.Any(), agentbinary.Version{
		Number: targetVersion,
		Arch:   arch.S390X,
	}).Return(nil, nil)
	s.agentBinaryService.EXPECT().RetrieveExternalAgentBinary(gomock.Any(), agentbinary.Version{
		Number: targetVersion,
		Arch:   arch.PPC64EL,
	}).DoAndReturn(func(ctx context.Context, v agentbinary.Version) (*service.ComputedHashes, error) {
		close(done)
		return nil, nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timeout waiting for agent binary retrieval")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestNewWorkerNoMissingArch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.modelAgentService.EXPECT().GetMissingAgentTargetVersions(gomock.Any()).DoAndReturn(func(ctx context.Context) (semversion.Number, []string, error) {
		close(done)
		return semversion.Zero, nil, nil
	})

	w := s.newWorker(c)
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatal("timeout waiting for agent binary retrieval")
	}

	workertest.CleanKill(c, w)
}

func (s *workerSuite) newWorker(c *tc.C) *updateWorker {
	return New(s.getConfig(c)).(*updateWorker)
}

func (s *workerSuite) getConfig(c *tc.C) WorkerConfig {
	return WorkerConfig{
		ModelAgentService:  s.modelAgentService,
		AgentBinaryService: s.agentBinaryService,
		Logger:             loggertesting.WrapCheckLog(c),
	}
}

func (s *workerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelAgentService = NewMockModelAgentService(ctrl)
	s.agentBinaryService = NewMockAgentBinaryService(ctrl)

	c.Cleanup(func() {
		s.modelAgentService = nil
		s.agentBinaryService = nil
	})

	return ctrl
}
