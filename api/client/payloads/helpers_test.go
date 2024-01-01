// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads

import (
	"github.com/juju/charm/v11"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/payloads"
	"github.com/juju/juju/rpc/params"
)

type helpersSuite struct {
}

var _ = gc.Suite(&helpersSuite{})

func (helpersSuite) TestPayload2api(c *gc.C) {
	apiPayload := Payload2api(payloads.FullPayloadInfo{
		Payload: payloads.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: payloads.StateRunning,
			Labels: []string{"a-tag"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	})

	c.Check(apiPayload, jc.DeepEquals, params.Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  payloads.StateRunning,
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
		Status:  payloads.StateRunning,
		Labels:  []string{"a-tag"},
		Unit:    names.NewUnitTag("a-application/0").String(),
		Machine: names.NewMachineTag("1").String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(pl, jc.DeepEquals, payloads.FullPayloadInfo{
		Payload: payloads.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: payloads.StateRunning,
			Labels: []string{"a-tag"},
			Unit:   "a-application/0",
		},
		Machine: "1",
	})
}
