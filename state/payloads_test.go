// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/component/all"
	"github.com/juju/juju/state"
	"github.com/juju/juju/workload"
)

func init() {
	if err := all.RegisterForServer(); err != nil {
		panic(err)
	}
}

var _ = gc.Suite(&envPayloadsSuite{})

type envPayloadsSuite struct {
	ConnSuite
}

const payloadsMetaYAML = `
name: a-charm
summary: a charm...
description: a charm...
payloads:
  workloadA:
    type: docker
`

func (s *envPayloadsSuite) addUnit(c *gc.C, charmName, serviceName, meta string) (names.CharmTag, *state.Unit) {
	ch := s.AddTestingCharm(c, charmName)
	ch = s.AddMetaCharm(c, charmName, meta, 2)

	svc := s.AddTestingService(c, serviceName, ch)
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	// TODO(ericsnow) Explicitly: call unit.AssignToMachine(m)?
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)

	charmTag := ch.Tag().(names.CharmTag)
	return charmTag, unit
}

func (s *envPayloadsSuite) TestFunctional(c *gc.C) {
	_, unit := s.addUnit(c, "dummy", "a-service", payloadsMetaYAML)
	ust, err := s.State.UnitPayloads(unit)
	c.Assert(err, jc.ErrorIsNil)

	st, err := s.State.EnvPayloads()
	c.Assert(err, jc.ErrorIsNil)

	payloads, err := st.ListAll()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(payloads, gc.HasLen, 0)

	err = ust.Track(workload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "payloadA",
			Type: "docker",
		},
		Status: workload.StateRunning,
		ID:     "xyz",
		Unit:   "a-service/0",
	})
	c.Assert(err, jc.ErrorIsNil)

	workloads, err := ust.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(workloads, gc.HasLen, 1)

	payloads, err = st.ListAll()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(payloads, jc.DeepEquals, []workload.FullPayloadInfo{{
		Payload: workload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "payloadA",
				Type: "docker",
			},
			ID:     "xyz",
			Status: workload.StateRunning,
			Labels: []string{},
			Unit:   "a-service/0",
		},
		Machine: "0",
	}})

	id, err := ust.LookUp("payloadA", "xyz")
	c.Assert(err, jc.ErrorIsNil)

	err = ust.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	payloads, err = st.ListAll()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(payloads, gc.HasLen, 0)
}
