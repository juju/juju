// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload/status"
)

var _ = gc.Suite(&formatterSuite{})

type formatterSuite struct{}

func (s *formatterSuite) TestFormatPayloadOkay(c *gc.C) {
	payload := status.NewPayload("spam", "a-service", 1, 0)
	payload.Tags = []string{"a-tag"}
	formatted := status.FormatPayload(payload)

	c.Check(formatted, jc.DeepEquals, status.FormattedPayload{
		Unit:    "a-service/0",
		Machine: "1",
		ID:      "idspam",
		Type:    "docker",
		Class:   "spam",
		Tags:    []string{"a-tag"},
		Status:  "running",
	})
}
