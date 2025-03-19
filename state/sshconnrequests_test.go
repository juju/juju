// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
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
	unit := s.Factory.MakeUnit(c, nil)
	err := s.State.InsertSSHConnRequest(
		state.SSHConnRequestArg{
			TunnelID:           "uuid",
			ModelUUID:          s.Model.UUID(),
			UnitName:           unit.Name(),
			Expires:            time.Now(),
			Username:           "test",
			Password:           "test-password",
			UnitPort:           22,
			EphemeralPublicKey: []byte{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.GetSSHConnRequest(state.SSHReqConnKeyID(unit.Name(), "uuid"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SSHReqConnReqSuite) TestRemoveSSHConnReq(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	err := s.State.InsertSSHConnRequest(
		state.SSHConnRequestArg{
			TunnelID:           "uuid",
			ModelUUID:          s.Model.UUID(),
			UnitName:           unit.Name(),
			Expires:            time.Now(),
			Username:           "test",
			Password:           "test-password",
			UnitPort:           22,
			EphemeralPublicKey: []byte{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	id := state.SSHReqConnKeyID(unit.Name(), "uuid")
	_, err = s.State.GetSSHConnRequest(id)
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveSSHConnRequest(state.SSHConnRequestRemoveArg{
		TunnelID:  "uuid",
		ModelUUID: s.Model.UUID(),
		UnitName:  unit.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	// assert it is actually deleted.
	err = s.sshconnreqs.FindId(id).One(&bson.D{})
	c.Assert(err, jc.ErrorIs, mgo.ErrNotFound)
}

func (s *SSHReqConnReqSuite) TestGetSSHConnReq(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	err := s.State.InsertSSHConnRequest(
		state.SSHConnRequestArg{
			TunnelID:           "uuid",
			ModelUUID:          s.Model.UUID(),
			UnitName:           unit.Name(),
			Expires:            time.Now(),
			Username:           "test",
			Password:           "test-password",
			UnitPort:           22,
			EphemeralPublicKey: []byte{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.GetSSHConnRequest(state.SSHReqConnKeyID(unit.Name(), "uuid"))
	c.Assert(err, jc.ErrorIsNil)

	req, err := s.State.GetSSHConnRequest(state.SSHReqConnKeyID(unit.Name(), "uuid"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(req.Username, gc.Equals, "test")
	c.Assert(req.Password, gc.Equals, "test-password")

	_, err = s.State.GetSSHConnRequest("not-found-docid")
	c.Assert(err, gc.ErrorMatches, "sshreqconn key \"not-found-docid\" not found")
}

func (s *SSHReqConnReqSuite) TestSSHConnReqWatcher(c *gc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  utils.RandomString(10, utils.LowerAlpha),
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: app})
	w := s.State.WatchSSHConnRequest(unit.Name())
	defer testing.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, w)

	// consume initial events
	wc.AssertChange()
	wc.AssertNoChange()

	// assert a connection request on another unit is not notified.
	err := s.State.InsertSSHConnRequest(
		state.SSHConnRequestArg{
			ModelUUID:          s.Model.UUID(),
			UnitName:           "other-unit",
			Expires:            time.Now(),
			Username:           "test",
			Password:           "test-password",
			UnitPort:           22,
			EphemeralPublicKey: []byte{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// assert a connection request regarding the right unit is notified.
	err = s.State.InsertSSHConnRequest(
		state.SSHConnRequestArg{
			TunnelID:           "uuid",
			ModelUUID:          s.Model.UUID(),
			UnitName:           unit.Name(),
			Expires:            time.Now(),
			Username:           "test",
			Password:           "test-password",
			UnitPort:           22,
			EphemeralPublicKey: []byte{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	id := state.SSHReqConnKeyID(unit.Name(), "uuid")
	wc.AssertChange(id)

	// assert deleting the document don't produce a notification.
	err = s.State.RemoveSSHConnRequest(state.SSHConnRequestRemoveArg{
		TunnelID:  "uuid",
		ModelUUID: s.Model.UUID(),
		UnitName:  unit.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}
