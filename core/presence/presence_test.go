// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence_test

import (
	"time"

	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/presence"
)

type suite struct{}

var _ = gc.Suite(&suite{})

func (*suite) assertEmptyConnections(c *gc.C, connections presence.Connections) {
	c.Assert(connections.Count(), gc.Equals, 0)
	c.Assert(connections.Models(), gc.HasLen, 0)
	c.Assert(connections.Servers(), gc.HasLen, 0)
	c.Assert(connections.Agents(), gc.HasLen, 0)
	c.Assert(connections.Values(), gc.HasLen, 0)
}

func (*suite) assertConnections(c *gc.C, connections presence.Connections, expected []presence.Value) {
	c.Assert(connections.Values(), jc.SameContents, expected)
}

func (s *suite) TestEmptyRecorder(c *gc.C) {
	r := presence.New(testclock.NewClock(time.Time{}))
	c.Assert(r.IsEnabled(), jc.IsFalse)
	r.Enable()
	s.assertEmptyConnections(c, r.Connections())
}

func (s *suite) TestBootstrapCase(c *gc.C) {
	r, _ := bootstrap()

	c.Assert(r.IsEnabled(), jc.IsTrue)

	connections := r.Connections()
	expected := []presence.Value{alive(ha0)}

	s.assertConnections(c, connections, expected)
	s.assertConnections(c, connections.ForModel(bootstrapUUID), expected)
	s.assertEmptyConnections(c, connections.ForModel(modelUUID))
}

func (s *suite) TestHAController(c *gc.C) {
	r, _ := bootstrap()
	enableHA(r)

	connections := r.Connections()
	expected := []presence.Value{alive(ha0), alive(ha1), alive(ha2)}

	c.Assert(connections.Values(), jc.DeepEquals, expected)
	s.assertConnections(c, connections.ForModel(bootstrapUUID), expected)
	s.assertEmptyConnections(c, connections.ForModel(modelUUID))

	s.assertConnections(c, connections.ForServer(ha0.Server), values(alive(ha0)))
	s.assertConnections(c, connections.ForServer(ha1.Server), values(alive(ha1)))
	s.assertConnections(c, connections.ForServer(ha2.Server), values(alive(ha2)))
}

func (s *suite) TestModels(c *gc.C) {
	r, _ := bootstrap()
	enableHA(r)
	deployModel(r)

	connections := r.Connections()
	s.assertConnections(c, connections.ForModel(bootstrapUUID),
		values(alive(ha0), alive(ha1), alive(ha2)))
	s.assertConnections(c, connections.ForModel(modelUUID),
		values(alive(modelMachine0), alive(modelMachine1),
			alive(modelUnit1), alive(modelUnit2)))

	s.assertConnections(c, connections.ForServer(ha0.Server),
		values(alive(ha0), alive(modelUnit1), alive(modelUnit2)))
	s.assertConnections(c, connections.ForServer(ha1.Server),
		values(alive(ha1), alive(modelMachine0)))
	s.assertConnections(c, connections.ForServer(ha2.Server),
		values(alive(ha2), alive(modelMachine1)))
}

func (s *suite) TestTimeRecording(c *gc.C) {
	now := time.Now()
	r, clock := bootstrap(now)

	m0 := lastSeen(alive(ha0), now)
	clock.Advance(time.Minute)
	now = clock.Now()
	enableHA(r)

	m1 := lastSeen(alive(ha1), now)
	m2 := lastSeen(alive(ha2), now)

	connections := r.Connections()
	s.assertConnections(c, connections.ForModel(bootstrapUUID),
		values(m0, m1, m2))
}

func (s *suite) TestActivity(c *gc.C) {
	r, clock := bootstrap()
	enableHA(r)
	deployModel(r)

	clock.Advance(5 * time.Minute)

	// Register activity for model machine 0.
	r.Activity("machine-1", 1237)

	// These connections are all the same except for modelMachine0
	// which shows an updated time.
	mm0 := lastSeen(alive(modelMachine0), clock.Now())

	connections := r.Connections()
	s.assertConnections(c, connections.ForModel(bootstrapUUID),
		values(alive(ha0), alive(ha1), alive(ha2)))
	s.assertConnections(c, connections.ForModel(modelUUID),
		values(mm0, alive(modelMachine1),
			alive(modelUnit1), alive(modelUnit2)))

	s.assertConnections(c, connections.ForServer(ha0.Server),
		values(alive(ha0), alive(modelUnit1), alive(modelUnit2)))
	s.assertConnections(c, connections.ForServer(ha1.Server),
		values(alive(ha1), mm0))
	s.assertConnections(c, connections.ForServer(ha2.Server),
		values(alive(ha2), alive(modelMachine1)))
}

func (s *suite) TestDisconnect(c *gc.C) {
	r, _ := bootstrap()
	enableHA(r)
	deployModel(r)

	r.Disconnect(ha0.Server, ha0.ConnectionID)
	r.Disconnect(modelUnit1.Server, modelUnit1.ConnectionID)

	connections := r.Connections()

	c.Assert(connections.Count(), gc.Equals, 5)
	s.assertConnections(c, connections.ForModel(bootstrapUUID),
		values(alive(ha1), alive(ha2)))
	s.assertConnections(c, connections.ForModel(modelUUID),
		values(alive(modelMachine0), alive(modelMachine1), alive(modelUnit2)))
}

func (s *suite) TestDisableClears(c *gc.C) {
	r, _ := bootstrap()
	r.Disable()

	s.assertEmptyConnections(c, r.Connections())
}

func (s *suite) TestServerDown(c *gc.C) {
	r, _ := bootstrap()
	enableHA(r)
	deployModel(r)

	r.ServerDown("machine-0")

	connections := r.Connections()
	s.assertConnections(c, connections.ForModel(bootstrapUUID),
		values(missing(ha0), alive(ha1), alive(ha2)))
	s.assertConnections(c, connections.ForModel(modelUUID),
		values(alive(modelMachine0), alive(modelMachine1),
			missing(modelUnit1), missing(modelUnit2)))

	s.assertConnections(c, connections.ForServer(ha0.Server),
		values(missing(ha0), missing(modelUnit1), missing(modelUnit2)))
	s.assertConnections(c, connections.ForServer(ha1.Server),
		values(alive(ha1), alive(modelMachine0)))
	s.assertConnections(c, connections.ForServer(ha2.Server),
		values(alive(ha2), alive(modelMachine1)))
}

func (s *suite) TestServerDownFollowedByConnections(c *gc.C) {
	r, _ := bootstrap()
	enableHA(r)
	deployModel(r)

	r.ServerDown("machine-0")
	connect(r, ha0)
	connect(r, modelUnit1)
	connect(r, modelUnit2)

	connections := r.Connections()
	s.assertConnections(c, connections.ForModel(bootstrapUUID),
		values(alive(ha0), alive(ha1), alive(ha2)))
	s.assertConnections(c, connections.ForModel(modelUUID),
		values(alive(modelMachine0), alive(modelMachine1),
			alive(modelUnit1), alive(modelUnit2)))

	s.assertConnections(c, connections.ForServer(ha0.Server),
		values(alive(ha0), alive(modelUnit1), alive(modelUnit2)))
	s.assertConnections(c, connections.ForServer(ha1.Server),
		values(alive(ha1), alive(modelMachine0)))
	s.assertConnections(c, connections.ForServer(ha2.Server),
		values(alive(ha2), alive(modelMachine1)))
}

func (s *suite) TestServerDownRaceConnections(c *gc.C) {
	r, _ := bootstrap()
	enableHA(r)
	deployModel(r)

	// Deal with the situation where machine-0 was marked as
	// down, but before an update for the server comes it, some
	// connections are updated, but the update from the server
	// hadn't processed all the connections yet.
	r.ServerDown("machine-0")
	connect(r, ha0)
	connect(r, modelUnit1)
	err := r.UpdateServer("machine-0", values(ha0))
	c.Assert(err, jc.ErrorIsNil)

	connections := r.Connections()
	s.assertConnections(c, connections.ForServer(ha0.Server),
		values(alive(ha0), alive(modelUnit1)))
}

func (s *suite) TestUpdateServer(c *gc.C) {
	r, _ := bootstrap()
	enableHA(r)
	deployModel(r)

	// Replace machine-0 values with just the ha node.
	// The values need to include the status of the connection.
	r.ServerDown("machine-0")
	err := r.UpdateServer("machine-0", values(ha0))
	c.Assert(err, jc.ErrorIsNil)

	connections := r.Connections()
	s.assertConnections(c, connections.ForModel(bootstrapUUID),
		values(alive(ha0), alive(ha1), alive(ha2)))
	s.assertConnections(c, connections.ForModel(modelUUID),
		values(alive(modelMachine0), alive(modelMachine1)))

	s.assertConnections(c, connections.ForServer(ha0.Server),
		values(alive(ha0)))
	s.assertConnections(c, connections.ForServer(ha1.Server),
		values(alive(ha1), alive(modelMachine0)))
	s.assertConnections(c, connections.ForServer(ha2.Server),
		values(alive(ha2), alive(modelMachine1)))
}

func (s *suite) TestUpdateServerError(c *gc.C) {
	r, _ := bootstrap()
	enableHA(r)
	deployModel(r)

	err := r.UpdateServer("machine-0", values(alive(ha1)))
	c.Assert(err, gc.ErrorMatches, `connection server mismatch, got "machine-1" expected "machine-0"`)
}

func (s *suite) TestConnections(c *gc.C) {
	r, _ := bootstrap()
	enableHA(r)
	deployModel(r)

	r.ServerDown("machine-0")

	connections := r.Connections()

	c.Assert(connections.Count(), gc.Equals, 7)
	status, err := connections.AgentStatus("machine-0")
	c.Assert(status, gc.Equals, presence.Unknown)
	c.Assert(err, gc.ErrorMatches, "connections not limited to a model, agent ambiguous")

	controllerConnections := connections.ForModel(bootstrapUUID)
	c.Assert(controllerConnections.Count(), gc.Equals, 3)

	status, err = controllerConnections.AgentStatus("machine-0")
	c.Assert(status, gc.Equals, presence.Missing)
	c.Assert(err, jc.ErrorIsNil)
	status, err = controllerConnections.AgentStatus("machine-1")
	c.Assert(status, gc.Equals, presence.Alive)
	c.Assert(err, jc.ErrorIsNil)
	status, err = controllerConnections.AgentStatus("machine-4")
	c.Assert(status, gc.Equals, presence.Unknown)
	c.Assert(err, jc.ErrorIsNil)
}

func bootstrap(initialTime ...time.Time) (presence.Recorder, *testclock.Clock) {
	if len(initialTime) > 1 {
		panic("initialTime should be zero or one values")
	}
	var t time.Time
	if len(initialTime) > 0 {
		t = initialTime[0]
	}
	// By using a testing clock with a zero time, the times are always empty.
	clock := testclock.NewClock(t)
	r := presence.New(clock)
	r.Enable()
	connect(r, ha0)
	return r, clock
}

func enableHA(r presence.Recorder) {
	connect(r, ha1)
	connect(r, ha2)
}

func deployModel(r presence.Recorder) {
	connect(r, modelMachine0)
	connect(r, modelMachine1)
	connect(r, modelUnit1)
	connect(r, modelUnit2)
}

func values(v ...presence.Value) []presence.Value {
	return v
}

func alive(v presence.Value) presence.Value {
	v.Status = presence.Alive
	return v
}

func missing(v presence.Value) presence.Value {
	v.Status = presence.Missing
	return v
}

func lastSeen(v presence.Value, when time.Time) presence.Value {
	v.LastSeen = when
	return v
}

func connect(r presence.Recorder, info presence.Value) {
	r.Connect(info.Server, info.Model, info.Agent, info.ConnectionID, info.ControllerAgent, info.UserData)
}

const bootstrapUUID = "bootstrap-uuid"
const modelUUID = "model-uuid"

var ha0 = presence.Value{
	Model:        bootstrapUUID,
	Server:       "machine-0",
	Agent:        "machine-0",
	ConnectionID: 1234,
}
var ha1 = presence.Value{
	Model:        bootstrapUUID,
	Server:       "machine-1",
	Agent:        "machine-1",
	ConnectionID: 1235,
}
var ha2 = presence.Value{
	Model:        bootstrapUUID,
	Server:       "machine-2",
	Agent:        "machine-2",
	ConnectionID: 1236,
}

var modelMachine0 = presence.Value{
	Model:        modelUUID,
	Server:       "machine-1",
	Agent:        "machine-0",
	ConnectionID: 1237,
}
var modelMachine1 = presence.Value{
	Model:        modelUUID,
	Server:       "machine-2",
	Agent:        "machine-1",
	ConnectionID: 1238,
}
var modelUnit1 = presence.Value{
	Model:        modelUUID,
	Server:       "machine-0",
	Agent:        "unit-wordpress-0",
	ConnectionID: 1239,
}
var modelUnit2 = presence.Value{
	Model:        modelUUID,
	Server:       "machine-0",
	Agent:        "unit-mysql-0",
	ConnectionID: 12409,
}
