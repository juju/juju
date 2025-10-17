// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/tools"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	mockModelState        *MockAgentBinaryModelState
	mockModelStore        *MockModelStore
	mockControllerState   *MockAgentBinaryControllerState
	mockControllerStore   *MockControllerStore
	mockSimpleStreamStore *MockSimpleStreamStore
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockModelState = NewMockAgentBinaryModelState(ctrl)
	s.mockModelStore = NewMockModelStore(ctrl)
	s.mockControllerState = NewMockAgentBinaryControllerState(ctrl)
	s.mockControllerStore = NewMockControllerStore(ctrl)
	s.mockSimpleStreamStore = NewMockSimpleStreamStore(ctrl)

	return ctrl
}

// TestListAgentBinaries tests the ListAgentBinaries method of the
// AgentBinaryService. It verifies that the method correctly merges
// agent binaries from the controller and model stores, with the model
// binaries taking precedence over the controller binaries.
func (s *serviceSuite) TestListAgentBinaries(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerBinaries := []agentbinary.Metadata{
		{
			Version: "4.0.0",
			Size:    1,
			SHA256:  "sha256hash-1",
		},
		{
			Version: "4.0.1",
			Size:    2,
			SHA256:  "sha256hash-2",
		},
	}
	modelBinaries := []agentbinary.Metadata{
		{
			Version: "4.0.1",
			Size:    222,
			// A same SHA should never have a different size, but this is just for testing the merge logic.
			SHA256: "sha256hash-2",
		},
		{
			Version: "4.0.2",
			Size:    3,
			SHA256:  "sha256hash-3",
		},
	}
	expected := []agentbinary.Metadata{
		{
			Version: "4.0.0",
			Size:    1,
			SHA256:  "sha256hash-1",
		},
		{
			Version: "4.0.1",
			Size:    222,
			SHA256:  "sha256hash-2",
		},
		{
			Version: "4.0.2",
			Size:    3,
			SHA256:  "sha256hash-3",
		},
	}
	s.mockControllerState.EXPECT().ListAgentBinaries(gomock.Any()).Return(controllerBinaries, nil)
	s.mockModelState.EXPECT().ListAgentBinaries(gomock.Any()).Return(modelBinaries, nil)

	svc := NewAgentBinaryService(s.mockModelState, s.mockModelStore, s.mockControllerState, s.mockControllerStore, s.mockSimpleStreamStore)
	result, err := svc.ListAgentBinaries(c.Context())
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.SameContents, expected)
}

func (s *serviceSuite) TestGetEnvironAgentBinariesFinder(c *tc.C) {
	finder := func(ctx context.Context, major, minor int, version semversion.Number, requestedStream string, filter tools.Filter) (tools.List, error) {
		return []*tools.Tools{
			&tools.Tools{
				Version: semversion.Binary{
					Number: semversion.Zero,
				},
				URL:    "test",
				SHA256: "sha256hash",
				Size:   4,
			},
		}, nil
	}
	s.mockSimpleStreamStore.EXPECT().GetEnvironAgentBinariesFinder().Return(finder)
	svc := NewAgentBinaryService(s.mockModelState, s.mockModelStore, s.mockControllerState, s.mockControllerStore, s.mockSimpleStreamStore)
	resultFinder := svc.GetEnvironAgentBinariesFinder()
	c.Assert(finder, tc.DeepEquals, resultFinder)
}
