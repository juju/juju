// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/testing"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type SSHReqConnReqSuite struct {
	ConnSuite
}

var _ = gc.Suite(&SSHReqConnReqSuite{})

func (s *SSHReqConnReqSuite) TestInsertSSHConnReq(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{InstanceId: "instance-id"})

	err := s.State.InsertSSHConnRequest(
		state.SSHConnRequestArg{
			TunnelID:           "uuid",
			ModelUUID:          s.Model.UUID(),
			MachineId:          machine.Id(),
			Expires:            time.Now(),
			Username:           "test",
			Password:           "test-password",
			UnitPort:           22,
			EphemeralPublicKey: []byte{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.GetSSHConnRequest(state.SSHReqConnKeyID(machine.Id(), "uuid"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SSHReqConnReqSuite) TestRemoveSSHConnReq(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{InstanceId: "instance-id"})

	err := s.State.InsertSSHConnRequest(
		state.SSHConnRequestArg{
			TunnelID:           "uuid",
			ModelUUID:          s.Model.UUID(),
			MachineId:          machine.Id(),
			Expires:            time.Now(),
			Username:           "test",
			Password:           "test-password",
			UnitPort:           22,
			EphemeralPublicKey: []byte{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	id := state.SSHReqConnKeyID(machine.Id(), "uuid")
	_, err = s.State.GetSSHConnRequest(id)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveSSHConnRequest(state.SSHConnRequestRemoveArg{
		TunnelID:  "uuid",
		ModelUUID: s.Model.UUID(),
		MachineId: machine.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	// assert it is actually deleted.
	err = s.sshconnreqs.FindId(id).One(&bson.D{})
	c.Assert(err, jc.ErrorIs, mgo.ErrNotFound)
}

func (s *SSHReqConnReqSuite) TestGetSSHConnReq(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{InstanceId: "instance-id"})

	err := s.State.InsertSSHConnRequest(
		state.SSHConnRequestArg{
			TunnelID:           "uuid",
			ModelUUID:          s.Model.UUID(),
			MachineId:          machine.Id(),
			Expires:            time.Now(),
			Username:           "test",
			Password:           "test-password",
			UnitPort:           22,
			EphemeralPublicKey: []byte{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.GetSSHConnRequest(state.SSHReqConnKeyID(machine.Id(), "uuid"))
	c.Assert(err, jc.ErrorIsNil)

	req, err := s.State.GetSSHConnRequest(state.SSHReqConnKeyID(machine.Id(), "uuid"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(req.Username, gc.Equals, "test")
	c.Assert(req.Password, gc.Equals, "test-password")

	_, err = s.State.GetSSHConnRequest("not-found-docid")
	c.Assert(err, gc.ErrorMatches, "sshreqconn key \"not-found-docid\" not found")
}

func (s *SSHReqConnReqSuite) TestSSHConnReqWatcher(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{InstanceId: "instance-id"})

	w := s.State.WatchSSHConnRequest(machine.Id())
	defer testing.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)

	// consume initial events
	wc.AssertChange()
	wc.AssertNoChange()

	// assert a connection request on another machine is not notified.
	err := s.State.InsertSSHConnRequest(
		state.SSHConnRequestArg{
			ModelUUID:          s.Model.UUID(),
			MachineId:          "other-machine",
			Expires:            time.Now(),
			Username:           "test",
			Password:           "test-password",
			UnitPort:           22,
			EphemeralPublicKey: []byte{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// assert a connection request regarding the right machine is notified.
	err = s.State.InsertSSHConnRequest(
		state.SSHConnRequestArg{
			TunnelID:           "uuid",
			ModelUUID:          s.Model.UUID(),
			MachineId:          machine.Id(),
			Expires:            time.Now(),
			Username:           "test",
			Password:           "test-password",
			UnitPort:           22,
			EphemeralPublicKey: []byte{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	id := state.SSHReqConnKeyID(machine.Id(), "uuid")
	wc.AssertChange(id)

	// assert deleting the document don't produce a notification.
	err = s.State.RemoveSSHConnRequest(state.SSHConnRequestRemoveArg{
		TunnelID:  "uuid",
		ModelUUID: s.Model.UUID(),
		MachineId: machine.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *SSHReqConnReqSuite) TestCleanupExpiredSSHConnRequest(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{InstanceId: "instance-id"})
	req1 := state.SSHConnRequestArg{
		TunnelID:           "tunnelid-1",
		ModelUUID:          s.Model.UUID(),
		MachineId:          machine.Id(),
		Expires:            time.Now().Add(-time.Minute),
		Username:           "test",
		Password:           "test-password",
		UnitPort:           22,
		EphemeralPublicKey: []byte{},
	}
	err := s.State.InsertSSHConnRequest(
		req1,
	)
	c.Assert(err, jc.ErrorIsNil)
	req2 := state.SSHConnRequestArg{
		TunnelID:           "tunnelid-2",
		ModelUUID:          s.Model.UUID(),
		MachineId:          machine.Id(),
		Expires:            time.Now().Add(time.Minute),
		Username:           "test",
		Password:           "test-password",
		UnitPort:           22,
		EphemeralPublicKey: []byte{},
	}
	err = s.State.InsertSSHConnRequest(
		req2,
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.Cleanup(nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.GetSSHConnRequest(state.SSHReqConnKeyID(req1.MachineId, req1.TunnelID))
	c.Assert(err, gc.ErrorMatches, "sshreqconn key \".*\" not found")
	_, err = s.State.GetSSHConnRequest(state.SSHReqConnKeyID(req2.MachineId, req2.TunnelID))
	c.Assert(err, jc.ErrorIsNil)
}
