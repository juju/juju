// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/version"
)

type MigrationSuite struct {
	ConnSuite
}

func (s *MigrationSuite) setLatestTools(c *gc.C, latestTools version.Number) {
	dbModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = dbModel.UpdateLatestToolsVersion(latestTools)
	c.Assert(err, jc.ErrorIsNil)
}

type MigrationExportSuite struct {
	MigrationSuite
}

var _ = gc.Suite(&MigrationExportSuite{})

func (s *MigrationExportSuite) TestModelInfo(c *gc.C) {
	latestTools := version.MustParse("2.0.1")
	s.setLatestTools(c, latestTools)
	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	dbModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Tag(), gc.Equals, dbModel.ModelTag())
	c.Assert(model.Owner(), gc.Equals, dbModel.Owner())
	config, err := dbModel.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Config(), jc.DeepEquals, config.AllAttrs())
	c.Assert(model.LatestToolsVersion(), gc.Equals, latestTools)
}

func (s *MigrationExportSuite) TestModelUsers(c *gc.C) {
	// Make sure we have some last connection times for the admin user,
	// and create a few other users.
	lastConnection := state.NowToTheSecond()
	owner, err := s.State.ModelUser(s.Owner)
	c.Assert(err, jc.ErrorIsNil)
	err = state.UpdateModelUserLastConnection(owner, lastConnection)
	c.Assert(err, jc.ErrorIsNil)

	bobTag := names.NewUserTag("bob@external")
	bob, err := s.State.AddModelUser(state.ModelUserSpec{
		User:      bobTag,
		CreatedBy: s.Owner,
		ReadOnly:  true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = state.UpdateModelUserLastConnection(bob, lastConnection)
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	users := model.Users()
	c.Assert(users, gc.HasLen, 2)

	exportedBob := users[0]
	// admin is "test-admin", and results are sorted
	exportedAdmin := users[1]

	c.Assert(exportedAdmin.Name(), gc.Equals, s.Owner)
	c.Assert(exportedAdmin.DisplayName(), gc.Equals, owner.DisplayName())
	c.Assert(exportedAdmin.CreatedBy(), gc.Equals, s.Owner)
	c.Assert(exportedAdmin.DateCreated(), gc.Equals, owner.DateCreated())
	c.Assert(exportedAdmin.LastConnection(), gc.Equals, lastConnection)
	c.Assert(exportedAdmin.ReadOnly(), jc.IsFalse)

	c.Assert(exportedBob.Name(), gc.Equals, bobTag)
	c.Assert(exportedBob.DisplayName(), gc.Equals, "")
	c.Assert(exportedBob.CreatedBy(), gc.Equals, s.Owner)
	c.Assert(exportedBob.DateCreated(), gc.Equals, bob.DateCreated())
	c.Assert(exportedBob.LastConnection(), gc.Equals, lastConnection)
	c.Assert(exportedBob.ReadOnly(), jc.IsTrue)
}

func (s *MigrationExportSuite) TestMachines(c *gc.C) {
	// Add a machine with an LXC container.
	machine1 := s.Factory.MakeMachine(c, nil)
	nested := s.Factory.MakeMachineNested(c, machine1.Id(), nil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	machines := model.Machines()
	c.Assert(machines, gc.HasLen, 1)

	exported := machines[0]
	c.Assert(exported.Tag(), gc.Equals, machine1.MachineTag())
	c.Assert(exported.Series(), gc.Equals, machine1.Series())
	tools, err := machine1.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	exTools := exported.Tools()
	c.Assert(exTools, gc.NotNil)
	c.Assert(exTools.Version(), jc.DeepEquals, tools.Version)

	containers := exported.Containers()
	c.Assert(containers, gc.HasLen, 1)
	container := containers[0]
	c.Assert(container.Tag(), gc.Equals, nested.MachineTag())
}

func (s *MigrationExportSuite) TestServices(c *gc.C) {
	service := s.Factory.MakeService(c, &factory.ServiceParams{
		Settings: map[string]interface{}{
			"foo": "bar",
		},
	})
	err := service.UpdateLeaderSettings(&goodToken{}, map[string]string{
		"leader": "true",
	})
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	services := model.Services()
	c.Assert(services, gc.HasLen, 1)

	exported := services[0]
	c.Assert(exported.Name(), gc.Equals, service.Name())
	c.Assert(exported.Tag(), gc.Equals, service.ServiceTag())
	c.Assert(exported.Series(), gc.Equals, service.Series())

	c.Assert(exported.Settings(), jc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
	c.Assert(exported.SettingsRefCount(), gc.Equals, 1)
	c.Assert(exported.LeadershipSettings(), jc.DeepEquals, map[string]interface{}{
		"leader": "true",
	})
}

func (s *MigrationExportSuite) TestMultipleServices(c *gc.C) {
	s.Factory.MakeService(c, &factory.ServiceParams{Name: "first"})
	s.Factory.MakeService(c, &factory.ServiceParams{Name: "second"})
	s.Factory.MakeService(c, &factory.ServiceParams{Name: "third"})

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	services := model.Services()
	c.Assert(services, gc.HasLen, 3)
}

type goodToken struct{}

// Check implements leadership.Token
func (*goodToken) Check(interface{}) error {
	return nil
}
