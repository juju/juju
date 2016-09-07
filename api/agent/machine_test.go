// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	apiserveragent "github.com/juju/juju/apiserver/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
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
	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)

	ssi := state.StateServingInfo{
		PrivateKey:   "some key",
		Cert:         "Some cert",
		SharedSecret: "really, really secret",
		APIPort:      33,
		StatePort:    44,
	}
	expected := params.StateServingInfo{
		PrivateKey:   ssi.PrivateKey,
		Cert:         ssi.Cert,
		SharedSecret: ssi.SharedSecret,
		APIPort:      ssi.APIPort,
		StatePort:    ssi.StatePort,
	}
	s.State.SetStateServingInfo(ssi)
	info, err := apiagent.NewState(st).StateServingInfo()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *servingInfoSuite) TestStateServingInfoPermission(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c)

	_, err := apiagent.NewState(st).StateServingInfo()
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "permission denied",
		Code:    "unauthorized access",
	})
}

func (s *servingInfoSuite) TestIsMaster(c *gc.C) {
	calledIsMaster := false
	var fakeMongoIsMaster = func(session *mgo.Session, m mongo.WithAddresses) (bool, error) {
		calledIsMaster = true
		return true, nil
	}
	s.PatchValue(&apiserveragent.MongoIsMaster, fakeMongoIsMaster)

	st, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	expected := true
	result, err := apiagent.NewState(st).IsMaster()

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, expected)
	c.Assert(calledIsMaster, jc.IsTrue)
}

func (s *servingInfoSuite) TestIsMasterPermission(c *gc.C) {
	st, _ := s.OpenAPIAsNewMachine(c)
	_, err := apiagent.NewState(st).IsMaster()
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: "permission denied",
		Code:    "unauthorized access",
	})
}

type machineSuite struct {
	testing.JujuConnSuite
	machine *state.Machine
	st      api.Connection
}

var _ = gc.Suite(&machineSuite{})

func (s *machineSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.st, s.machine = s.OpenAPIAsNewMachine(c)
}

func (s *machineSuite) TestMachineEntity(c *gc.C) {
	tag := names.NewMachineTag("42")
	m, err := apiagent.NewState(s.st).Entity(tag)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(m, gc.IsNil)

	m, err = apiagent.NewState(s.st).Entity(s.machine.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Tag(), gc.Equals, s.machine.Tag().String())
	c.Assert(m.Life(), gc.Equals, params.Alive)
	c.Assert(m.Jobs(), gc.DeepEquals, []multiwatcher.MachineJob{multiwatcher.JobHostUnits})

	err = s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove()
	c.Assert(err, jc.ErrorIsNil)

	m, err = apiagent.NewState(s.st).Entity(s.machine.Tag())
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("machine %s not found", s.machine.Id()))
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
	c.Assert(m, gc.IsNil)
}

func (s *machineSuite) TestEntitySetPassword(c *gc.C) {
	entity, err := apiagent.NewState(s.st).Entity(s.machine.Tag())
	c.Assert(err, jc.ErrorIsNil)

	err = entity.SetPassword("foo")
	c.Assert(err, gc.ErrorMatches, "password is only 3 bytes long, and is not a valid Agent password")
	err = entity.SetPassword("foo-12345678901234567890")
	c.Assert(err, jc.ErrorIsNil)
	err = entity.ClearReboot()
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.machine.PasswordValid("bar"), jc.IsFalse)
	c.Assert(s.machine.PasswordValid("foo-12345678901234567890"), jc.IsTrue)

	// Check that we cannot log in to mongo with the correct password.
	// This is because there's no mongo password set for s.machine,
	// which has JobHostUnits
	info := s.MongoInfo(c)
	// TODO(dfc) this entity.Tag should return a Tag
	tag, err := names.ParseTag(entity.Tag())
	c.Assert(err, jc.ErrorIsNil)
	info.Tag = tag
	info.Password = "foo-12345678901234567890"
	err = tryOpenState(s.State.ModelTag(), s.State.ControllerTag(), info)
	c.Assert(errors.Cause(err), jc.Satisfies, errors.IsUnauthorized)
}

func (s *machineSuite) TestClearReboot(c *gc.C) {
	err := s.machine.SetRebootFlag(true)
	c.Assert(err, jc.ErrorIsNil)
	rFlag, err := s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rFlag, jc.IsTrue)

	entity, err := apiagent.NewState(s.st).Entity(s.machine.Tag())
	c.Assert(err, jc.ErrorIsNil)

	err = entity.ClearReboot()
	c.Assert(err, jc.ErrorIsNil)

	rFlag, err = s.machine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rFlag, jc.IsFalse)
}

func tryOpenState(modelTag names.ModelTag, controllerTag names.ControllerTag, info *mongo.MongoInfo) error {
	st, err := state.Open(modelTag, controllerTag, info, mongotest.DialOpts(), nil)
	if err == nil {
		st.Close()
	}
	return err
}
