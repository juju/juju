// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/client"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
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
	tag := names.NewUserTag("admin")
	authorizer := &apiservertesting.FakeAuthorizer{Tag: tag}
	var err error
	s.api, err = client.NewClient(
		s.st,
		nil, // storage
		nil, // pool
		nil, // resources
		authorizer,
		nil, // presence
		nil, // toolsFinder
		nil, // newEnviron
		nil, // blockChecker
		nil,
		nil, // multiwatcher.Factory
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
}

func statusInfoWithDates(si []status.StatusInfo) []status.StatusInfo {
	// Add timestamps to input status info records.
	// Timestamps will be in descending order so that we can
	// check that sorting has occurred and the output should
	// be in ascending order.
	result := make([]status.StatusInfo, len(si))
	for i, s := range si {
		t := time.Unix(int64(1000-i), 0)
		s.Since = &t
		result[i] = s
	}
	return result
}

func reverseStatusInfo(si []status.StatusInfo) []status.StatusInfo {
	result := make([]status.StatusInfo, len(si))
	for i, s := range si {
		result[len(si)-i-1] = s
	}
	return result
}

func checkStatusInfo(c *gc.C, obtained []params.DetailedStatus, expected []status.StatusInfo) {
	c.Assert(len(obtained), gc.Equals, len(expected))
	lastTimestamp := int64(0)
	for i, obtainedInfo := range obtained {
		c.Logf("Checking status %q with info %q", obtainedInfo.Status, obtainedInfo.Info)
		thisTimeStamp := obtainedInfo.Since.Unix()
		c.Assert(thisTimeStamp >= lastTimestamp, jc.IsTrue)
		lastTimestamp = thisTimeStamp
		obtainedInfo.Since = nil
		c.Assert(obtainedInfo.Status, gc.Equals, expected[i].Status.String())
		c.Assert(obtainedInfo.Info, gc.Equals, expected[i].Message)
	}
}

func (s *statusHistoryTestSuite) TestSizeRequired(c *gc.C) {
	r := s.api.StatusHistory(params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Tag:    "unit-unit-1",
			Kind:   status.KindUnit.String(),
			Filter: params.StatusHistoryFilter{Size: 0},
		}}})
	c.Assert(r.Results, gc.HasLen, 1)
	c.Assert(r.Results[0].Error.Message, gc.Equals, "cannot validate status history filter: missing filter parameters not valid")
}

func (s *statusHistoryTestSuite) TestNoConflictingFilters(c *gc.C) {
	now := time.Now()
	r := s.api.StatusHistory(params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Tag:    "unit-unit-1",
			Kind:   status.KindUnit.String(),
			Filter: params.StatusHistoryFilter{Size: 1, Date: &now},
		}}})
	c.Assert(r.Results, gc.HasLen, 1)
	c.Assert(r.Results[0].Error.Message, gc.Equals, "cannot validate status history filter: Size and Date together not valid")

	yesterday := time.Hour * 24
	r = s.api.StatusHistory(params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Tag:    "unit-unit-1",
			Kind:   status.KindUnit.String(),
			Filter: params.StatusHistoryFilter{Size: 1, Delta: &yesterday},
		}}})
	c.Assert(r.Results, gc.HasLen, 1)
	c.Assert(r.Results[0].Error.Message, gc.Equals, "cannot validate status history filter: Size and Delta together not valid")

	r = s.api.StatusHistory(params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Tag:    "unit-unit-1",
			Kind:   status.KindUnit.String(),
			Filter: params.StatusHistoryFilter{Date: &now, Delta: &yesterday},
		}}})
	c.Assert(r.Results, gc.HasLen, 1)
	c.Assert(r.Results[0].Error.Message, gc.Equals, "cannot validate status history filter: Date and Delta together not valid")
}

func (s *statusHistoryTestSuite) TestStatusHistoryApplication(c *gc.C) {
	s.st.appHistory = statusInfoWithDates([]status.StatusInfo{
		{
			Status:  status.Maintenance,
			Message: "working",
		},
		{
			Status:  status.Active,
			Message: "running",
		},
	})
	h := s.api.StatusHistory(params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Tag:    "application-app",
			Kind:   status.KindApplication.String(),
			Filter: params.StatusHistoryFilter{Size: 10},
		}}})
	c.Assert(h.Results, gc.HasLen, 1)
	c.Assert(h.Results[0].Error, gc.IsNil)
	checkStatusInfo(c, h.Results[0].History.Statuses, reverseStatusInfo(s.st.appHistory))
}

func (s *statusHistoryTestSuite) TestStatusHistoryUnitOnly(c *gc.C) {
	s.st.unitHistory = statusInfoWithDates([]status.StatusInfo{
		{
			Status:  status.Maintenance,
			Message: "working",
		},
		{
			Status:  status.Active,
			Message: "running",
		},
	})
	s.st.agentHistory = statusInfoWithDates([]status.StatusInfo{
		{
			Status: status.Idle,
		},
	})
	h := s.api.StatusHistory(params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Tag:    "unit-unit-0",
			Kind:   status.KindWorkload.String(),
			Filter: params.StatusHistoryFilter{Size: 10},
		}}})
	c.Assert(h.Results, gc.HasLen, 1)
	c.Assert(h.Results[0].Error, gc.IsNil)
	checkStatusInfo(c, h.Results[0].History.Statuses, reverseStatusInfo(s.st.unitHistory))
}

func (s *statusHistoryTestSuite) TestStatusHistoryAgentOnly(c *gc.C) {
	s.st.unitHistory = statusInfoWithDates([]status.StatusInfo{
		{
			Status:  status.Maintenance,
			Message: "working",
		},
		{
			Status:  status.Active,
			Message: "running",
		},
	})
	s.st.agentHistory = statusInfoWithDates([]status.StatusInfo{
		{
			Status: status.Executing,
		},
		{
			Status: status.Idle,
		},
	})
	h := s.api.StatusHistory(params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Tag:    "unit-unit-0",
			Kind:   status.KindUnitAgent.String(),
			Filter: params.StatusHistoryFilter{Size: 10},
		}}})
	c.Assert(h.Results, gc.HasLen, 1)
	c.Assert(h.Results[0].Error, gc.IsNil)
	checkStatusInfo(c, h.Results[0].History.Statuses, reverseStatusInfo(s.st.agentHistory))
}

func (s *statusHistoryTestSuite) TestStatusHistoryCombined(c *gc.C) {
	s.st.unitHistory = statusInfoWithDates([]status.StatusInfo{
		{
			Status:  status.Maintenance,
			Message: "working",
		},
		{
			Status:  status.Active,
			Message: "running",
		},
		{
			Status:  status.Blocked,
			Message: "waiting",
		},
	})
	s.st.agentHistory = statusInfoWithDates([]status.StatusInfo{
		{
			Status: status.Executing,
		},
		{
			Status: status.Idle,
		},
	})
	h := s.api.StatusHistory(params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Tag:    "unit-unit-0",
			Kind:   status.KindUnit.String(),
			Filter: params.StatusHistoryFilter{Size: 3},
		}}})
	c.Assert(h.Results, gc.HasLen, 1)
	c.Assert(h.Results[0].Error, gc.IsNil)
	expected := []status.StatusInfo{
		s.st.agentHistory[1],
		s.st.unitHistory[0],
		s.st.agentHistory[0],
	}
	checkStatusInfo(c, h.Results[0].History.Statuses, expected)
}

func (s *statusHistoryTestSuite) TestStatusHistoryModelOnly(c *gc.C) {
	s.st.modelHistory = statusInfoWithDates([]status.StatusInfo{
		{
			Status:  status.Active,
			Message: "all ok",
		},
		{
			Status:  status.Suspended,
			Message: "invalid creds",
		},
	})
	h := s.api.StatusHistory(params.StatusHistoryRequests{
		Requests: []params.StatusHistoryRequest{{
			Tag:    "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
			Kind:   status.KindModel.String(),
			Filter: params.StatusHistoryFilter{Size: 10},
		}}})
	c.Assert(h.Results, gc.HasLen, 1)
	c.Assert(h.Results[0].Error, gc.IsNil)
	checkStatusInfo(c, h.Results[0].History.Statuses, reverseStatusInfo(s.st.modelHistory))
}

type mockState struct {
	client.Backend
	appHistory   []status.StatusInfo
	unitHistory  []status.StatusInfo
	agentHistory []status.StatusInfo
	modelHistory []status.StatusInfo
}

func (m *mockState) Model() (client.Model, error) {
	return &mockModel{status: m.modelHistory}, nil
}

func (m *mockState) ModelUUID() string {
	return "uuid"
}

func (m *mockState) ModelTag() names.ModelTag {
	return names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f00d")
}

func (m *mockState) ControllerTag() names.ControllerTag {
	return names.NewControllerTag("deadbeef-0bad-400d-8000-4b1d0d06f00d")
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

func (m *mockState) Application(name string) (client.Application, error) {
	if name != "app" {
		return nil, errors.NotFoundf("%v", name)
	}
	return &mockApplication{
		status: m.appHistory,
	}, nil
}

type mockModel struct {
	status statuses
	client.Model
}

func (m mockModel) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	return m.status.StatusHistory(filter)
}

type mockApplication struct {
	status statuses
	client.Application
}

func (m *mockApplication) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	return m.status.StatusHistory(filter)
}

type mockUnit struct {
	status statuses
	agent  *mockUnitAgent
	client.Unit
}

func (m *mockUnit) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	return m.status.StatusHistory(filter)
}

func (m *mockUnit) AgentHistory() status.StatusHistoryGetter {
	return m.agent
}

type mockUnitAgent struct {
	statuses
}

type statuses []status.StatusInfo

func (s statuses) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	if filter.Size > len(s) {
		filter.Size = len(s)
	}
	return s[:filter.Size], nil
}
