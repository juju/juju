// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

type helpersSuite struct{}

var _ = gc.Suite(&helpersSuite{})

func (helpersSuite) TestPayload2api(c *gc.C) {
	apiPayload := Payload2api(workload.FullPayloadInfo{
		Payload: workload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: workload.StateRunning,
			Tags:   []string{"a-tag"},
			Unit:   "a-service/0",
		},
		Machine: "1",
	})

	c.Check(apiPayload, jc.DeepEquals, Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  workload.StateRunning,
		Tags:    []string{"a-tag"},
		Unit:    "a-service/0",
		Machine: "1",
	})
}

func (helpersSuite) TestAPI2Payload(c *gc.C) {
	payload := API2Payload(Payload{
		Class:   "spam",
		Type:    "docker",
		ID:      "idspam",
		Status:  workload.StateRunning,
		Tags:    []string{"a-tag"},
		Unit:    "a-service/0",
		Machine: "1",
	})

	c.Check(payload, jc.DeepEquals, workload.FullPayloadInfo{
		Payload: workload.Payload{
			PayloadClass: charm.PayloadClass{
				Name: "spam",
				Type: "docker",
			},
			ID:     "idspam",
			Status: workload.StateRunning,
			Tags:   []string{"a-tag"},
			Unit:   "a-service/0",
		},
		Machine: "1",
	})
}
