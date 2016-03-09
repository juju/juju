// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&statusHistoryTestSuite{})

type statusHistoryTestSuite struct {
	testing.BaseSuite
	st  *mockState
	api *client.Client
}

func (s *statusHistoryTestSuite) SetUpTest(c *gc.C) {
	s.st = &mockState{}
	client.PatchState(s, s.st)
	tag := names.NewUserTag("user")
	authorizer := &apiservertesting.FakeAuthorizer{Tag: tag}
	var err error
	s.api, err = client.NewClient(nil, nil, authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func statusInfoWithDates(si []state.StatusInfo) []state.StatusInfo {
	// Add timestamps to input status info records.
	// Timestamps will be in descending order so that we can
	// check that sorting has occurred and the output should
	// be in ascending order.
	result := make([]state.StatusInfo, len(si))
	for i, s := range si {
		t := time.Unix(int64(1000-i), 0)
		s.Since = &t
		result[i] = s
	}
	return result
}

func reverseStatusInfo(si []state.StatusInfo) []state.StatusInfo {
	result := make([]state.StatusInfo, len(si))
	for i, s := range si {
		result[len(si)-i-1] = s
	}
	return result
}

func checkStatusInfo(c *gc.C, obtained []params.AgentStatus, expected []state.StatusInfo) {
	c.Assert(len(obtained), gc.Equals, len(expected))
	lastTimestamp := int64(0)
	for i, obtainedInfo := range obtained {
		thisTimeStamp := obtainedInfo.Since.Unix()
		c.Assert(thisTimeStamp >= lastTimestamp, jc.IsTrue)
		lastTimestamp = thisTimeStamp
		obtainedInfo.Since = nil
		c.Assert(obtainedInfo.Status, gc.Equals, params.Status(expected[i].Status))
		c.Assert(obtainedInfo.Info, gc.Equals, expected[i].Message)
	}
}

func (s *statusHistoryTestSuite) TestSizeRequired(c *gc.C) {
	_, err := s.api.UnitStatusHistory(params.StatusHistory{
		Name: "unit",
		Kind: params.KindCombined,
		Size: 0,
	})
	c.Assert(err, gc.ErrorMatches, "invalid history size: 0")
}

func (s *statusHistoryTestSuite) TestStatusHistoryUnitOnly(c *gc.C) {
	s.st.unitHistory = statusInfoWithDates([]state.StatusInfo{
		{
			Status:  state.StatusMaintenance,
			Message: "working",
		},
		{
			Status:  state.StatusActive,
			Message: "running",
		},
	})
	s.st.agentHistory = statusInfoWithDates([]state.StatusInfo{
		{
			Status: state.StatusIdle,
		},
	})
	h, err := s.api.UnitStatusHistory(params.StatusHistory{
		Name: "unit/0",
		Kind: params.KindWorkload,
		Size: 10,
	})
	c.Assert(err, jc.ErrorIsNil)
	checkStatusInfo(c, h.Statuses, reverseStatusInfo(s.st.unitHistory))
}

func (s *statusHistoryTestSuite) TestStatusHistoryAgentOnly(c *gc.C) {
	s.st.unitHistory = statusInfoWithDates([]state.StatusInfo{
		{
			Status:  state.StatusMaintenance,
			Message: "working",
		},
		{
			Status:  state.StatusActive,
			Message: "running",
		},
	})
	s.st.agentHistory = statusInfoWithDates([]state.StatusInfo{
		{
			Status: state.StatusExecuting,
		},
		{
			Status: state.StatusIdle,
		},
	})
	h, err := s.api.UnitStatusHistory(params.StatusHistory{
		Name: "unit/0",
		Kind: params.KindAgent,
		Size: 10,
	})
	c.Assert(err, jc.ErrorIsNil)
	checkStatusInfo(c, h.Statuses, reverseStatusInfo(s.st.agentHistory))
}

func (s *statusHistoryTestSuite) TestStatusHistoryCombined(c *gc.C) {
	s.st.unitHistory = statusInfoWithDates([]state.StatusInfo{
		{
			Status:  state.StatusMaintenance,
			Message: "working",
		},
		{
			Status:  state.StatusActive,
			Message: "running",
		},
		{
			Status:  state.StatusBlocked,
			Message: "waiting",
		},
	})
	s.st.agentHistory = statusInfoWithDates([]state.StatusInfo{
		{
			Status: state.StatusExecuting,
		},
		{
			Status: state.StatusIdle,
		},
	})
	h, err := s.api.UnitStatusHistory(params.StatusHistory{
		Name: "unit/0",
		Kind: params.KindCombined,
		Size: 3,
	})
	c.Assert(err, jc.ErrorIsNil)
	expected := []state.StatusInfo{
		s.st.agentHistory[1],
		s.st.unitHistory[0],
		s.st.agentHistory[0],
	}
	checkStatusInfo(c, h.Statuses, expected)
}

type mockState struct {
	client.StateInterface
	unitHistory  []state.StatusInfo
	agentHistory []state.StatusInfo
}

func (m *mockState) ModelUUID() string {
	return "uuid"
}

func (m *mockState) Unit(name string) (client.Unit, error) {
	if name != "unit/0" {
		return nil, errors.NotFoundf("%v", name)
	}
	return &mockUnit{
		status: m.unitHistory,
		agent:  &mockUnitAgent{m.agentHistory},
	}, nil
}

type mockUnit struct {
	status statuses
	agent  *mockUnitAgent
	client.Unit
}

func (m *mockUnit) StatusHistory(size int) ([]state.StatusInfo, error) {
	return m.status.StatusHistory(size)
}

func (m *mockUnit) AgentHistory() state.StatusHistoryGetter {
	return m.agent
}

type mockUnitAgent struct {
	statuses
}

type statuses []state.StatusInfo

func (s statuses) StatusHistory(size int) ([]state.StatusInfo, error) {
	if size > len(s) {
		size = len(s)
	}
	return s[:size], nil
}
