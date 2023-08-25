// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	jujutesting "github.com/juju/juju/testing"
)

type EncodeToStringSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&EncodeToStringSuite{})

func (s *EncodeToStringSuite) TestEncodeToString(c *gc.C) {
	cfg := jujutesting.FakeControllerConfig()

	encoded, err := controller.EncodeToString(cfg)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(encoded, gc.DeepEquals, map[string]string{
		"controller-uuid":           jujutesting.ControllerTag.Id(),
		"ca-cert":                   jujutesting.CACert,
		"state-port":                "1234",
		"api-port":                  "17777",
		"set-numa-control-policy":   "false",
		"model-logfile-max-backups": "1",
		"model-logfile-max-size":    "1M",
		"model-logs-size":           "1M",
		"max-txn-log-size":          "10M",
		"auditing-enabled":          "false",
		"audit-log-capture-args":    "true",
		"audit-log-max-size":        "200M",
		"audit-log-max-backups":     "5",
		"query-tracing-threshold":   "1s",
	})
}
