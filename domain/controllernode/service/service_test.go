// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	state *MockState
}

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}

func (s *serviceSuite) TestUpdateExternalControllerSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().CurateNodes(gomock.Any(), []string{"3", "4"}, []string{"1"})

	err := NewService(s.state).CurateNodes(c.Context(), []string{"3", "4"}, []string{"1"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateDqliteNode(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().UpdateDqliteNode(gomock.Any(), "0", uint64(12345), "192.168.5.60")

	err := NewService(s.state).UpdateDqliteNode(c.Context(), "0", 12345, "192.168.5.60")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestIsModelKnownToController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	knownID := "known"
	fakeID := "fake"

	exp := s.state.EXPECT()
	gomock.InOrder(
		exp.SelectDatabaseNamespace(gomock.Any(), fakeID).Return("", controllernodeerrors.NotFound),
		exp.SelectDatabaseNamespace(gomock.Any(), knownID).Return(knownID, nil),
	)

	svc := NewService(s.state)

	known, err := svc.IsKnownDatabaseNamespace(c.Context(), fakeID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(known, tc.IsFalse)

	known, err = svc.IsKnownDatabaseNamespace(c.Context(), knownID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(known, tc.IsTrue)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.ARM64,
	}

	s.state.EXPECT().SetRunningAgentBinaryVersion(gomock.Any(), controllerID, ver).Return(nil)

	svc := NewService(s.state)
	err := svc.SetControllerNodeReportedAgentVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := "1"

	ver := coreagentbinary.Version{
		Number: semversion.Zero,
	}
	err := svc.SetControllerNodeReportedAgentVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIs, errors.NotValid)

	ver = coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.UnsupportedArches[0],
	}
	err = svc.SetControllerNodeReportedAgentVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestSetControllerNodeAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := "1"
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("1.2.3"),
		Arch:   corearch.ARM64,
	}

	s.state.EXPECT().SetRunningAgentBinaryVersion(gomock.Any(), controllerID, ver).Return(controllernodeerrors.NotFound)

	err := svc.SetControllerNodeReportedAgentVersion(
		c.Context(),
		controllerID,
		ver,
	)
	c.Assert(err, tc.ErrorIs, controllernodeerrors.NotFound)
}

func (s *serviceSuite) TestIsControllerNode(c *tc.C) {
	defer s.setupMocks(c).Finish()

	controllerID := "1"
	fakeID := "fake"

	exp := s.state.EXPECT()
	gomock.InOrder(
		exp.IsControllerNode(gomock.Any(), fakeID).Return(false, controllernodeerrors.NotFound),
		exp.IsControllerNode(gomock.Any(), controllerID).Return(true, nil),
	)

	svc := NewService(s.state)

	is, err := svc.IsControllerNode(c.Context(), fakeID)
	c.Assert(err, tc.ErrorIs, controllernodeerrors.NotFound)
	c.Check(is, tc.IsFalse)

	is, err = svc.IsControllerNode(c.Context(), controllerID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(is, tc.IsTrue)
}

func (s *serviceSuite) TestIsControllerNodeNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.state)

	controllerID := ""

	is, err := svc.IsControllerNode(c.Context(), controllerID)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
	c.Check(is, tc.IsFalse)
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
