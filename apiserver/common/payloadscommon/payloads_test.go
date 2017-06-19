// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloadscommon_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common/payloadscommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
)

type PayloadsHelpersSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&PayloadsHelpersSuite{})

func (PayloadsHelpersSuite) TestPayloadToParam(c *gc.C) {
	apiPayload := payloadscommon.PayloadInfoToParams(payload.FullPayloadInfo{
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
