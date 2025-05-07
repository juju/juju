// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/controller"
	jujutesting "github.com/juju/juju/internal/testing"
)

type EncodeToStringSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&EncodeToStringSuite{})

func (s *EncodeToStringSuite) TestEncodeToString(c *tc.C) {
	cfg := jujutesting.FakeControllerConfig()

	encoded, err := controller.EncodeToString(cfg)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(encoded, tc.DeepEquals, map[string]string{
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
		"object-store-type":         "file",
	})
}
