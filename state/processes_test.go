// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/component/all"
	"github.com/juju/juju/process"
	"github.com/juju/juju/state"
)

func init() {
	if err := all.RegisterForServer(); err != nil {
		panic(err)
	}
}

var _ = gc.Suite(&unitProcessesSuite{})

type unitProcessesSuite struct {
	ConnSuite
}

const metaYAML = `
name: a-charm
summary: a charm...
description: a charm...
processes:
  procA:
    type: docker
    command: do-something cool
    image: spam/eggs
    env:
      IMPORTANT: YES
`

func (s *unitProcessesSuite) addUnit(c *gc.C, charmName, serviceName, meta string) (names.CharmTag, *state.Unit) {
	ch := s.AddTestingCharm(c, charmName)
	ch = s.AddMetaCharm(c, charmName, meta, 2)

	svc := s.AddTestingService(c, serviceName, ch)
	unit, err := svc.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	charmTag := ch.Tag().(names.CharmTag)
	return charmTag, unit
}

func (s *unitProcessesSuite) TestFunctional(c *gc.C) {
	_, unit := s.addUnit(c, "dummy", "a-service", metaYAML)

	st, err := s.State.UnitProcesses(unit)
	c.Assert(err, jc.ErrorIsNil)

	procs, err := st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(procs, jc.DeepEquals, []process.Info{{
		Process: charm.Process{
			Name:    "procA",
			Type:    "docker",
			Command: "do-something cool",
			Image:   "spam/eggs",
			EnvVars: map[string]string{
				// TODO(erisnow) YAML coerces YES into true...
				"IMPORTANT": "true",
			},
		},
	}})

	info := process.Info{
		Process: charm.Process{
			Name:    "procA",
			Type:    "docker",
			Command: "do-something cool",
			Image:   "spam/eggs",
			EnvVars: map[string]string{
				"IMPORTANT": "true",
			},
		},
		Details: process.Details{
			ID: "xyz",
			Status: process.Status{
				Label: "running",
			},
		},
	}
	err = st.Register(info)
	c.Assert(err, jc.ErrorIsNil)

	procs, err = st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(procs, jc.DeepEquals, []process.Info{info})

	procs, err = st.List("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(procs, jc.DeepEquals, []process.Info{info})

	err = st.SetStatus("procA/xyz", process.Status{Label: "still running"})
	c.Assert(err, jc.ErrorIsNil)

	procs, err = st.List("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(procs, jc.DeepEquals, []process.Info{{
		Process: charm.Process{
			Name:    "procA",
			Type:    "docker",
			Command: "do-something cool",
			Image:   "spam/eggs",
			EnvVars: map[string]string{
				"IMPORTANT": "true",
			},
		},
		Details: process.Details{
			ID: "xyz",
			Status: process.Status{
				Label: "still running",
			},
		},
	}})

	err = st.Unregister("procA/xyz")
	c.Assert(err, jc.ErrorIsNil)

	procs, err = st.List()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(procs, jc.DeepEquals, []process.Info{{
		Process: charm.Process{
			Name:    "procA",
			Type:    "docker",
			Command: "do-something cool",
			Image:   "spam/eggs",
			EnvVars: map[string]string{
				"IMPORTANT": "true",
			},
		},
	}})
}
