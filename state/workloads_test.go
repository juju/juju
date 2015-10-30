// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/state"
	"github.com/juju/juju/workload"
)

var _ = gc.Suite(&unitWorkloadsSuite{})

type unitWorkloadsSuite struct {
	ConnSuite
}

const workloadsMetaYAML = `
name: a-charm
summary: a charm...
description: a charm...
workloads:
  workloadA:
    type: docker
    command: do-something cool
    image: spam/eggs
    ports:
      - 8080:80
    volumes:
      - /var/nginx/html:/usr/share/nginx/html:ro
    env:
      IMPORTANT: YES
`

func (s *unitWorkloadsSuite) addUnit(c *gc.C, charmName, serviceName, meta string) (names.CharmTag, *state.Unit) {
	ch := s.AddTestingCharm(c, charmName)
	ch = s.AddMetaCharm(c, charmName, meta, 2)

	svc := s.AddTestingService(c, serviceName, ch)
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	charmTag := ch.Tag().(names.CharmTag)
	return charmTag, unit
}

func (s *unitWorkloadsSuite) TestFunctional(c *gc.C) {
	_, unit := s.addUnit(c, "dummy", "a-service", workloadsMetaYAML)

	st, err := s.State.UnitWorkloads(unit)
	c.Assert(err, jc.ErrorIsNil)

	workloads, err := st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(workloads, gc.HasLen, 0)

	info := workload.Info{
		PayloadClass: charm.PayloadClass{
			Name: "workloadA",
			Type: "docker",
		},
		Status: workload.Status{
			State:   workload.StateRunning,
			Message: "okay",
		},
		Details: workload.Details{
			ID: "xyz",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}
	err = st.Track(info)
	c.Assert(err, jc.ErrorIsNil)

	workloads, err = st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(workloads, jc.DeepEquals, []workload.Info{{
		PayloadClass: charm.PayloadClass{
			Name: "workloadA",
			Type: "docker",
		},
		Status: workload.Status{
			State:   workload.StateRunning,
			Message: "okay",
		},
		Details: workload.Details{
			ID: "xyz",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}})

	id, err := st.LookUp("workloadA", "xyz")
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("using ID %q", id)

	workloads, err = st.List(id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(workloads, jc.DeepEquals, []workload.Info{info})

	err = st.SetStatus(id, "running")
	c.Assert(err, jc.ErrorIsNil)

	workloads, err = st.List(id)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(workloads, jc.DeepEquals, []workload.Info{{
		PayloadClass: charm.PayloadClass{
			Name: "workloadA",
			Type: "docker",
		},
		Status: workload.Status{
			State:   workload.StateRunning,
			Message: "running",
		},
		Details: workload.Details{
			ID: "xyz",
			Status: workload.PluginStatus{
				State: "running",
			},
		},
	}})

	err = st.Untrack(id)
	c.Assert(err, jc.ErrorIsNil)

	workloads, err = st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(workloads, gc.HasLen, 0)
}
