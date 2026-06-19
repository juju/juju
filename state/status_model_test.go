// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/charm/v12"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type ModelStatusSuite struct {
	ConnSuite
	st      *state.State
	model   *state.Model
	factory *factory.Factory
}

var _ = gc.Suite(&ModelStatusSuite{})

func (s *ModelStatusSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.st = s.Factory.MakeModel(c, nil)
	m, err := s.st.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.model = m
	s.factory = factory.NewFactory(s.st, s.StatePool)
}

func (s *ModelStatusSuite) TearDownTest(c *gc.C) {
	if s.st != nil {
		err := s.st.Close()
		c.Assert(err, jc.ErrorIsNil)
		s.st = nil
	}
	s.ConnSuite.TearDownTest(c)
}

func (s *ModelStatusSuite) TestInitialStatus(c *gc.C) {
	s.checkInitialStatus(c)
}

func (s *ModelStatusSuite) checkInitialStatus(c *gc.C) {
	statusInfo, err := s.model.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Available)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, gc.HasLen, 0)
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *ModelStatusSuite) TestSetUnknownStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.model.SetStatus(sInfo)
	c.Assert(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *ModelStatusSuite) TestSetOverwritesData(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Available,
		Message: "blah",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.model.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ModelStatusSuite) TestGetSetStatusDying(c *gc.C) {
	// Add a machine to the model to ensure it is non-empty
	// when we destroy; this prevents the model from advancing
	// directly to Dead.
	s.factory.MakeMachine(c, nil)

	err := s.model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ModelStatusSuite) TestGetSetStatusDead(c *gc.C) {
	err := s.model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c)
}

func (s *ModelStatusSuite) TestGetSetStatusGone(c *gc.C) {
	err := s.model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.st.RemoveDyingModel()
	c.Assert(err, jc.ErrorIsNil)

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Available,
		Message: "not really",
		Since:   &now,
	}
	err = s.model.SetStatus(sInfo)
	c.Check(err, gc.ErrorMatches, `cannot set status: model not found`)

	_, err = s.model.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: model not found`)
}

func (s *ModelStatusSuite) checkGetSetStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Available,
		Message: "blah",
		Data: map[string]interface{}{
			"$foo.bar.baz": map[string]interface{}{
				"pew.pew": "zap",
			}},
		Since: &now,
	}
	err := s.model.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)

	// Get another instance of the Model to compare against
	model, err := s.st.Model()
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := model.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Available)
	c.Check(statusInfo.Message, gc.Equals, "blah")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{
		"$foo.bar.baz": map[string]interface{}{
			"pew.pew": "zap",
		},
	})
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *ModelStatusSuite) TestModelStatusForModel(c *gc.C) {
	ms, err := s.model.LoadModelStatus()
	c.Assert(err, jc.ErrorIsNil)

	info, err := ms.Model()
	c.Assert(err, jc.ErrorIsNil)

	mInfo, err := s.model.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, mInfo)
}

func (s *ModelStatusSuite) TestApplicationStatus(c *gc.C) {
	application := s.factory.MakeApplication(c, nil)
	now := testing.ZeroTime()
	err := application.SetStatus(status.StatusInfo{
		Status:  status.Active,
		Message: "healthy",
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)

	ms, err := s.model.LoadModelStatus()
	c.Assert(err, jc.ErrorIsNil)

	msStatus, err := ms.Application(application.Name())
	c.Assert(err, jc.ErrorIsNil)
	appStatus, err := application.Status()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(msStatus, jc.DeepEquals, appStatus)
}

func (s *ModelStatusSuite) TestApplicationOperatorStatus(c *gc.C) {
	st := s.factory.MakeModel(c, &factory.ModelParams{
		Name: "caas-model",
		Type: state.ModelTypeCAAS,
	})
	defer st.Close()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	application := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})

	now := testing.ZeroTime()
	err = application.SetOperatorStatus(status.StatusInfo{
		Status:  status.Error,
		Message: "broken",
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)

	ms, err := model.LoadModelStatus()
	c.Assert(err, jc.ErrorIsNil)

	msStatus, err := ms.ApplicationOperator(application.Name())
	c.Assert(err, jc.ErrorIsNil)
	appStatus, err := application.OperatorStatus()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(msStatus, jc.DeepEquals, appStatus)
}

func (s *ModelStatusSuite) TestRelationStatus(c *gc.C) {
	relation := s.factory.MakeRelation(c, nil)
	err := relation.SetStatus(status.StatusInfo{
		Status:  status.Suspended,
		Message: "for a while",
	})
	c.Assert(err, jc.ErrorIsNil)

	ms, err := s.model.LoadModelStatus()
	c.Assert(err, jc.ErrorIsNil)

	msStatus, err := ms.Relation(relation.Id())
	c.Assert(err, jc.ErrorIsNil)
	relStatus, err := relation.Status()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(msStatus, jc.DeepEquals, relStatus)
}

func (s *ModelStatusSuite) TestRemoteApplicationStatus(c *gc.C) {
	application, err := s.st.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "hosted-mysql",
		OfferUUID:   "offer-uuid",
		URL:         "fred/prod.mysql",
		SourceModel: s.model.ModelTag(),
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	now := testing.ZeroTime()
	err = application.SetStatus(status.StatusInfo{
		Status:  status.Active,
		Message: "ready",
		Since:   &now,
	})
	c.Assert(err, jc.ErrorIsNil)

	ms, err := s.model.LoadModelStatus()
	c.Assert(err, jc.ErrorIsNil)

	msStatus, err := ms.RemoteApplication(application.Name())
	c.Assert(err, jc.ErrorIsNil)
	appStatus, err := application.Status()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(msStatus, jc.DeepEquals, appStatus)
}

func (s *ModelStatusSuite) TestMachineStatus(c *gc.C) {
	machine := s.factory.MakeMachine(c, nil)

	ms, err := s.model.LoadModelStatus()
	c.Assert(err, jc.ErrorIsNil)

	msAgent, err := ms.MachineAgent(machine.Id())
	c.Assert(err, jc.ErrorIsNil)
	msInstance, err := ms.MachineInstance(machine.Id())
	c.Assert(err, jc.ErrorIsNil)

	mAgent, err := machine.Status()
	c.Assert(err, jc.ErrorIsNil)
	mInstance, err := machine.InstanceStatus()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(msAgent, jc.DeepEquals, mAgent)
	c.Assert(msInstance, jc.DeepEquals, mInstance)
}

func (s *ModelStatusSuite) TestUnitStatus(c *gc.C) {
	unit := s.factory.MakeUnit(c, nil)

	c.Assert(unit.SetWorkloadVersion("42.1"), jc.ErrorIsNil)
	c.Assert(unit.SetStatus(status.StatusInfo{Status: status.Active}), jc.ErrorIsNil)
	c.Assert(unit.SetAgentStatus(status.StatusInfo{Status: status.Idle}), jc.ErrorIsNil)

	ms, err := s.model.LoadModelStatus()
	c.Assert(err, jc.ErrorIsNil)

	msAgent, err := ms.UnitAgent(unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	msWorkload, err := ms.UnitWorkload(unit.Name(), true)
	c.Assert(err, jc.ErrorIsNil)
	msWorkloadVersion, err := ms.UnitWorkloadVersion(unit.Name())
	c.Assert(err, jc.ErrorIsNil)

	uAgent, err := unit.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	uWorkload, err := unit.Status()
	c.Assert(err, jc.ErrorIsNil)
	uWorkloadVersion, err := unit.WorkloadVersion()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(msAgent, jc.DeepEquals, uAgent)
	c.Check(msWorkload, jc.DeepEquals, uWorkload)
	c.Check(msWorkloadVersion, jc.DeepEquals, uWorkloadVersion)
}

func (s *ModelStatusSuite) TestUnitStatusWeirdness(c *gc.C) {
	unit := s.factory.MakeUnit(c, nil)

	// When the agent status is in error, we show the workload status
	// as an error, and the agent as idle
	c.Assert(unit.SetStatus(status.StatusInfo{Status: status.Active}), jc.ErrorIsNil)
	c.Assert(unit.SetAgentStatus(status.StatusInfo{
		Status:  status.Error,
		Message: "OMG"}), jc.ErrorIsNil)

	ms, err := s.model.LoadModelStatus()
	c.Assert(err, jc.ErrorIsNil)

	msAgent, err := ms.UnitAgent(unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	msWorkload, err := ms.UnitWorkload(unit.Name(), true)
	c.Assert(err, jc.ErrorIsNil)

	uAgent, err := unit.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	uWorkload, err := unit.Status()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(msAgent, jc.DeepEquals, uAgent)
	c.Check(msWorkload, jc.DeepEquals, uWorkload)

	c.Check(msAgent.Status, gc.Equals, status.Idle)
	c.Check(msWorkload.Status, gc.Equals, status.Error)
}
