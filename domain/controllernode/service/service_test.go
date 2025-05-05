// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	internalerrors "github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}

func (s *serviceSuite) TestUpdateExternalControllerSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CurateNodes(gomock.Any(), []string{"3", "4"}, []string{"1"})

	err := NewService(s.state).CurateNodes(context.Background(), []string{"3", "4"}, []string{"1"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateDqliteNode(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().UpdateDqliteNode(gomock.Any(), "0", uint64(12345), "192.168.5.60")

	err := NewService(s.state).UpdateDqliteNode(context.Background(), "0", 12345, "192.168.5.60")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestIsModelKnownToController(c *gc.C) {
	defer s.setupMocks(c).Finish()

	knownID := "known"
	fakeID := "fake"

	exp := s.state.EXPECT()
	gomock.InOrder(
		exp.SelectDatabaseNamespace(gomock.Any(), fakeID).Return("", controllernodeerrors.NotFound),
		exp.SelectDatabaseNamespace(gomock.Any(), knownID).Return(knownID, nil),
	)

	svc := NewService(s.state)

	known, err := svc.IsKnownDatabaseNamespace(context.Background(), fakeID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(known, jc.IsFalse)

	known, err = svc.IsKnownDatabaseNamespace(context.Background(), knownID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(known, jc.IsTrue)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.ARM64,
	}

	s.state.EXPECT().SetRunningAgentBinaryVersion(gomock.Any(), controllerID, ver).Return(nil)

	svc := NewService(s.state)
	err := svc.SetControllerNodeReportedAgentVersion(
		context.Background(),
		controllerID,
		ver,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := "1"

	ver := coreagentbinary.Version{
		Number: semversion.Zero,
	}
	err := svc.SetControllerNodeReportedAgentVersion(
		context.Background(),
		controllerID,
		ver,
	)
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	ver = coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.UnsupportedArches[0],
	}
	err = svc.SetControllerNodeReportedAgentVersion(
		context.Background(),
		controllerID,
		ver,
	)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.ARM64,
	}

	s.state.EXPECT().SetRunningAgentBinaryVersion(gomock.Any(), controllerID, ver).Return(controllernodeerrors.NotFound)

	err := svc.SetControllerNodeReportedAgentVersion(
		context.Background(),
		controllerID,
		ver,
	)
	c.Assert(err, jc.ErrorIs, controllernodeerrors.NotFound)
}

func (s *serviceSuite) TestSetAPIAddressNotValid(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := "1"
	address := "1:2:3"

	err := svc.SetAPIAddress(context.Background(), controllerID, address, true)
	c.Assert(err, jc.ErrorIs, controllernodeerrors.ControllerAddressNotValid)
}

func (s *serviceSuite) TestSetAPIAddressNotValidIPv4MissingPort(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := "1"
	address := "1.2.3.4"

	err := svc.SetAPIAddress(context.Background(), controllerID, address, true)
	c.Assert(err, jc.ErrorIs, controllernodeerrors.ControllerAddressNotValid)
}

func (s *serviceSuite) TestSetAPIAddressNotValidIPv6MissingPort(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := "1"
	address := "[1:2:3:4:5:6:7:8]"

	err := svc.SetAPIAddress(context.Background(), controllerID, address, true)
	c.Assert(err, jc.ErrorIs, controllernodeerrors.ControllerAddressNotValid)
}

func (s *serviceSuite) TestSetAPIAddressStateError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := "1"
	address := "1.2.3.4:1234"

	s.state.EXPECT().SetAPIAddress(gomock.Any(), controllerID, address, true).Return(internalerrors.New("boom"))

	err := svc.SetAPIAddress(context.Background(), controllerID, address, true)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestSetAPIAddressState(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := "1"
	address := "1.2.3.4:1234"

	s.state.EXPECT().SetAPIAddress(gomock.Any(), controllerID, address, true).Return(nil)

	err := svc.SetAPIAddress(context.Background(), controllerID, address, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetControllerIDs(c *gc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	s.state.EXPECT().GetControllerIDs(gomock.Any()).Return([]string{"1", "2"}, nil)

	controllerIDs, err := svc.GetControllerIDs(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerIDs, gc.HasLen, 2)
	c.Check(controllerIDs, gc.DeepEquals, []string{"1", "2"})
}
