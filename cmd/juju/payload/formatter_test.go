// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/payload"
)

var _ = gc.Suite(&formatterSuite{})

type formatterSuite struct {
	testing.IsolationSuite
}

func (s *formatterSuite) TestFormatPayloadOkay(c *gc.C) {
	pl := payload.NewPayload("spam", "a-application", 1, 0)
	pl.Labels = []string{"a-tag"}
	formatted := payload.FormatPayload(pl)

	c.Check(formatted, jc.DeepEquals, payload.FormattedPayload{
		Unit:    "a-application/0",
		Machine: "1",
		ID:      "idspam",
		Type:    "docker",
		Class:   "spam",
		Labels:  []string{"a-tag"},
		Status:  "running",
	})
}
