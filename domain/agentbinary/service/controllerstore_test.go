// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"io"
	"strings"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/semversion"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type controllerStoreSuite struct {
	testhelpers.IsolationSuite

	mockState             *MockControllerStoreState
	mockObjectStoreGetter *MockModelObjectStoreGetter
	mockObjectStore       *MockObjectStore
}

func TestControllerStoreSuite(t *testing.T) {
	tc.Run(t, &controllerStoreSuite{})
}

func (s *controllerStoreSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockControllerStoreState(ctrl)
	s.mockObjectStore = NewMockObjectStore(ctrl)
	s.mockObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil).AnyTimes()
	return ctrl
}

func (s *controllerStoreSuite) TestGetAgentBinary(c *tc.C) {
	defer s.setupMocks(c).Finish()
	sum := "439c9ea02f8561c5a152d7cf4818d72cd5f2916b555d82c5eee599f5e8f3d09e"

	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.6.8"),
		Arch:   corearch.AMD64,
	}

	s.mockState.EXPECT().GetAgentBinarySHA256(gomock.Any(), ver, coreagentbinary.AgentStreamProposed).Return(true, sum, nil)
	agentBinary := strings.NewReader("test-agent-binary")
	data := io.NopCloser(agentBinary)
	s.mockObjectStore.EXPECT().GetBySHA256(gomock.Any(), sum).Return(
		data, agentBinary.Size(), nil,
	)

	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil)

	store := NewControllerAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)

	reader, size, sha256Str, err := store.GetAgentBinary(c.Context(), ver, coreagentbinary.AgentStreamProposed)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reader, gc.Equals, data)
	c.Assert(size, gc.Equals, agentBinary.Size())
	c.Assert(sha256Str, gc.Equals, sum)
}

func (s *controllerStoreSuite) TestGetAgentBinaryNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.6.8"),
		Arch:   corearch.AMD64,
	}

	s.mockState.EXPECT().GetAgentBinarySHA256(gomock.Any(), ver, coreagentbinary.AgentStreamProposed).Return(false, "", nil)

	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil)
	store := NewControllerAgentBinaryStore(s.mockState, loggertesting.WrapCheckLog(c), s.mockObjectStoreGetter)

	reader, size, sha256Str, err := store.GetAgentBinary(c.Context(), ver, coreagentbinary.AgentStreamProposed)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reader, gc.Equals, nil)
	c.Assert(size, gc.Equals, 0)
	c.Assert(sha256Str, gc.Equals, "")
}
