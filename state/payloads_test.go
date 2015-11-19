// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/component/all"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/state"
)

func init() {
	if err := all.RegisterForServer(); err != nil {
		panic(err)
	}
}

var (
	_ = gc.Suite(&envPayloadsSuite{})
	_ = gc.Suite(&unitPayloadsSuite{})
)

type envPayloadsSuite struct {
	ConnSuite
}

func (s *envPayloadsSuite) TestFunctional(c *gc.C) {
	machine := "0"
	unit := addUnit(c, s.ConnSuite, unitArgs{
		charm:    "dummy",
		service:  "a-service",
		metadata: payloadsMetaYAML,
		machine:  machine,
	})

	ust, err := s.State.UnitPayloads(unit)
	c.Assert(err, jc.ErrorIsNil)

	st, err := s.State.EnvPayloads()
	c.Assert(err, jc.ErrorIsNil)

	payloads, err := st.ListAll()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(payloads, gc.HasLen, 0)

	err = ust.Track(payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "payloadA",
			Type: "docker",
		},
		Status: payload.StateRunning,
		ID:     "xyz",
		Unit:   "a-service/0",
	})
	c.Assert(err, jc.ErrorIsNil)

	unitPayloads, err := ust.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitPayloads, gc.HasLen, 1)

	payloads, err = st.ListAll()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(payloads, jc.DeepEquals, []payload.FullPayloadInfo{{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "payloadA",
				Type: "docker",
			},
			ID:     "xyz",
			Status: payload.StateRunning,
			Labels: []string{},
			Unit:   "a-service/0",
		},
		Machine: machine,
	}})

	id, err := ust.LookUp("payloadA", "xyz")
	c.Assert(err, jc.ErrorIsNil)

	err = ust.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	payloads, err = st.ListAll()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(payloads, gc.HasLen, 0)
}

type unitPayloadsSuite struct {
	ConnSuite
}

func (s *unitPayloadsSuite) TestFunctional(c *gc.C) {
	machine := "0"
	unit := addUnit(c, s.ConnSuite, unitArgs{
		charm:    "dummy",
		service:  "a-service",
		metadata: payloadsMetaYAML,
		machine:  machine,
	})

	st, err := s.State.UnitPayloads(unit)
	c.Assert(err, jc.ErrorIsNil)

	results, err := st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 0)

	pl := payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "payloadA",
			Type: "docker",
		},
		ID:     "xyz",
		Status: payload.StateRunning,
		Unit:   "a-service/0",
	}
	err = st.Track(pl)
	c.Assert(err, jc.ErrorIsNil)

	results, err = st.List()
	c.Assert(err, jc.ErrorIsNil)
	// TODO(ericsnow) Once Track returns the new ID we can drop
	// the following two lines.
	c.Assert(results, gc.HasLen, 1)
	id := results[0].ID
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID: id,
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: machine,
		},
	}})

	lookedUpID, err := st.LookUp("payloadA", "xyz")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(lookedUpID, gc.Equals, id)

	c.Logf("using ID %q", id)
	results, err = st.List(id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID: id,
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: machine,
		},
	}})

	err = st.SetStatus(id, "running")
	c.Assert(err, jc.ErrorIsNil)

	results, err = st.List(id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, []payload.Result{{
		ID: id,
		Payload: &payload.FullPayloadInfo{
			Payload: pl,
			Machine: machine,
		},
	}})

	// Ensure duplicates are not allowed.
	err = st.Track(pl)
	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
	results, err = st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 1)

	err = st.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	results, err = st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, gc.HasLen, 0)
}

const payloadsMetaYAML = `
name: a-charm
summary: a charm...
description: a charm...
payloads:
  payloadA:
    type: docker
`

type unitArgs struct {
	charm    string
	service  string
	metadata string
	machine  string
}

func addUnit(c *gc.C, s ConnSuite, args unitArgs) *state.Unit {
	ch := s.AddTestingCharm(c, args.charm)
	ch = s.AddMetaCharm(c, args.charm, args.metadata, 2)

	svc := s.AddTestingService(c, args.service, ch)
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// TODO(ericsnow) Explicitly: call unit.AssignToMachine(m)?
	c.Assert(args.machine, gc.Equals, "0")
	err = unit.AssignToNewMachine() // machine "0"
	c.Assert(err, jc.ErrorIsNil)

	return unit
}
