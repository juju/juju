// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/mgo.v2"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api"
	apiserveragent "github.com/juju/juju/apiserver/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type servingInfoSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&servingInfoSuite{})

func (s *servingInfoSuite) TestStateServingInfo(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)

	expected := state.StateServingInfo{
		PrivateKey:   "some key",
		Cert:         "Some cert",
		SharedSecret: "really, really secret",
		APIPort:      33,
		StatePort:    44,
	}
	s.State.SetStateServingInfo(expected)
	info, err := st.Agent().StateServingInfo()
	c.Assert(err, gc.IsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *servingInfoSuite) TestStateServingInfoPermission(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c)

	_, err := st.Agent().StateServingInfo()
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *servingInfoSuite) TestIsMaster(c *gc.C) {
	calledIsMaster := false
	var fakeMongoIsMaster = func(session *mgo.Session, m mongo.WithAddresses) (bool, error) {
		calledIsMaster = true
		return true, nil
	}
	s.PatchValue(&apiserveragent.MongoIsMaster, fakeMongoIsMaster)

	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	expected := true
	result, err := st.Agent().IsMaster()

	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, expected)
	c.Assert(calledIsMaster, gc.Equals, true)
}

func (s *servingInfoSuite) TestIsMasterPermission(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c)
	_, err := st.Agent().IsMaster()
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

type machineSuite struct {
	testing.JujuConnSuite
	machine *state.Machine
	st      *api.State
}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.st, s.machine = s.OpenAPIAsNewMachine(c)
}

func (s *machineSuite) TestMachineEntity(c *gc.C) {
	tag := names.NewMachineTag("42")
	m, err := s.st.Agent().Entity(tag)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(m, gc.IsNil)

	m, err = s.st.Agent().Entity(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(m.Tag(), gc.Equals, s.machine.Tag().String())
	c.Assert(m.Life(), gc.Equals, params.Alive)
	c.Assert(m.Jobs(), gc.DeepEquals, []params.MachineJob{params.JobHostUnits})

	err = s.machine.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machine.Remove()
	c.Assert(err, gc.IsNil)

	m, err = s.st.Agent().Entity(s.machine.Tag())
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("machine %s not found", s.machine.Id()))
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(m, gc.IsNil)
}

func (s *machineSuite) TestEntitySetPassword(c *gc.C) {
	entity, err := s.st.Agent().Entity(s.machine.Tag())
	c.Assert(err, gc.IsNil)

	err = entity.SetPassword("foo")
	c.Assert(err, gc.ErrorMatches, "password is only 3 bytes long, and is not a valid Agent password")
	err = entity.SetPassword("foo-12345678901234567890")
	c.Assert(err, gc.IsNil)

	err = s.machine.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machine.PasswordValid("bar"), gc.Equals, false)
	c.Assert(s.machine.PasswordValid("foo-12345678901234567890"), gc.Equals, true)

	// Check that we cannot log in to mongo with the correct password.
	// This is because there's no mongo password set for s.machine,
	// which has JobHostUnits
	info := s.MongoInfo(c)
	// TODO(dfc) this entity.Tag should return a Tag
	tag, err := names.ParseTag(entity.Tag())
	c.Assert(err, gc.IsNil)
	info.Tag = tag
	info.Password = "foo-12345678901234567890"
	err = tryOpenState(info)
	c.Assert(err, jc.Satisfies, errors.IsUnauthorized)
}

func tryOpenState(info *mongo.MongoInfo) error {
	st, err := state.Open(info, mongo.DialOpts{}, environs.NewStatePolicy())
	if err == nil {
		st.Close()
	}
	return err
}
