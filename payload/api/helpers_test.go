// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
)

type helpersSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&helpersSuite{})

func (helpersSuite) TestPayload2api(c *gc.C) {
	apiPayload := Payload2api(payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: payload.StateRunning,
			Labels: []string{"a-tag"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	})

	c.Check(apiPayload, jc.DeepEquals, params.Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  payload.StateRunning,
		Labels:  []string{"a-tag"},
		Unit:    names.NewUnitTag("a-application/0").String(),
		Machine: names.NewMachineTag("1").String(),
	})
}

func (helpersSuite) TestAPI2Payload(c *gc.C) {
	pl, err := API2Payload(params.Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  payload.StateRunning,
		Labels:  []string{"a-tag"},
		Unit:    names.NewUnitTag("a-application/0").String(),
		Machine: names.NewMachineTag("1").String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(pl, jc.DeepEquals, payload.FullPayloadInfo{
		Payload: payload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: payload.StateRunning,
			Labels: []string{"a-tag"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	})
}
