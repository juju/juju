// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/api"
)

var _ = gc.Suite(&publicSuite{})

type publicSuite struct {
	testing.IsolationSuite

	stub  *testing.Stub
	state *stubState
}

func (s *publicSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.state = &stubState{stub: s.stub}
}

func (publicSuite) newPayload(name string) (payload.FullPayloadInfo, api.Payload) {
	ptype := "docker"
	id := "id" + name
	tags := []string{"a-tag"}
	unit := "a-service/0"
	machine := "1"

	pl := payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: ptype,
			},
			ID:     id,
			Status: payload.StateRunning,
			Labels: tags,
			Unit:   unit,
		},
		Machine: machine,
	}
	apiPayload := api.Payload{
		Class:   name,
		Type:    ptype,
		ID:      id,
		Status:  payload.StateRunning,
		Labels:  tags,
		Unit:    names.NewUnitTag(unit).String(),
		Machine: names.NewMachineTag(machine).String(),
	}
	return pl, apiPayload
}

func (s *publicSuite) TestListNoPatterns(c *gc.C) {
	payloadA, apiPayloadA := s.newPayload("spam")
	payloadB, apiPayloadB := s.newPayload("eggs")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB)

	facade := PublicAPI{s.state}
	args := api.EnvListArgs{
		Patterns: []string{},
	}
	results, err := facade.List(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.EnvListResults{
		Results: []api.Payload{
			apiPayloadA,
			apiPayloadB,
		},
	})
}

func (s *publicSuite) TestListAllMatch(c *gc.C) {
	payloadA, apiPayloadA := s.newPayload("spam")
	payloadB, apiPayloadB := s.newPayload("eggs")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB)

	facade := PublicAPI{s.state}
	args := api.EnvListArgs{
		Patterns: []string{
			"a-service/0",
		},
	}
	results, err := facade.List(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.EnvListResults{
		Results: []api.Payload{
			apiPayloadA,
			apiPayloadB,
		},
	})
}

func (s *publicSuite) TestListNoMatch(c *gc.C) {
	payloadA, _ := s.newPayload("spam")
	payloadB, _ := s.newPayload("eggs")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB)

	facade := PublicAPI{s.state}
	args := api.EnvListArgs{
		Patterns: []string{
			"a-service/1",
		},
	}
	results, err := facade.List(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results.Results, gc.HasLen, 0)
}

func (s *publicSuite) TestListNoPayloads(c *gc.C) {
	facade := PublicAPI{s.state}
	args := api.EnvListArgs{
		Patterns: []string{},
	}
	results, err := facade.List(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results.Results, gc.HasLen, 0)
}

func (s *publicSuite) TestListMultiMatch(c *gc.C) {
	payloadA, apiPayloadA := s.newPayload("spam")
	payloadB, apiPayloadB := s.newPayload("eggs")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB)

	facade := PublicAPI{s.state}
	args := api.EnvListArgs{
		Patterns: []string{
			"spam",
			"eggs",
		},
	}
	results, err := facade.List(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.EnvListResults{
		Results: []api.Payload{
			apiPayloadA,
			apiPayloadB,
		},
	})
}

func (s *publicSuite) TestListPartialMatch(c *gc.C) {
	payloadA, apiPayloadA := s.newPayload("spam")
	payloadB, _ := s.newPayload("eggs")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB)

	facade := PublicAPI{s.state}
	args := api.EnvListArgs{
		Patterns: []string{
			"spam",
		},
	}
	results, err := facade.List(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.EnvListResults{
		Results: []api.Payload{
			apiPayloadA,
		},
	})
}

func (s *publicSuite) TestListPartialMultiMatch(c *gc.C) {
	payloadA, apiPayloadA := s.newPayload("spam")
	payloadB, _ := s.newPayload("eggs")
	payloadC, apiPayloadC := s.newPayload("ham")
	s.state.payloads = append(s.state.payloads, payloadA, payloadB, payloadC)

	facade := PublicAPI{s.state}
	args := api.EnvListArgs{
		Patterns: []string{
			"spam",
			"ham",
		},
	}
	results, err := facade.List(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, api.EnvListResults{
		Results: []api.Payload{
			apiPayloadA,
			apiPayloadC,
		},
	})
}

func (s *publicSuite) TestListAllFilters(c *gc.C) {
	pl := payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: payload.StateRunning,
			Labels: []string{"a-tag"},
			Unit:   "a-service/0",
		},
		Machine: "1",
	}
	apiPayload := api.Payload2api(pl)
	s.state.payloads = append(s.state.payloads, pl)

	facade := PublicAPI{s.state}
	patterns := []string{
		"spam",               // name
		"docker",             // type
		"idspam",             // ID
		payload.StateRunning, // status
		"a-service/0",        // unit
		"1",                  // machine
		"a-tag",              // tags
	}
	for _, pattern := range patterns {
		c.Logf("trying pattern %q", pattern)

		args := api.EnvListArgs{
			Patterns: []string{
				pattern,
			},
		}
		results, err := facade.List(args)
		c.Assert(err, jc.ErrorIsNil)

		c.Check(results, jc.DeepEquals, api.EnvListResults{
			Results: []api.Payload{
				apiPayload,
			},
		})
	}
}

type stubState struct {
	stub *testing.Stub

	payloads []payload.FullPayloadInfo
}

func (s *stubState) ListAll() ([]payload.FullPayloadInfo, error) {
	s.stub.AddCall("ListAll")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.payloads, nil
}
