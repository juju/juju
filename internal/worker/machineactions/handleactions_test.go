// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/exec"

	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/machineactions"
)

type HandleSuite struct {
	testhelpers.IsolationSuite
}

func TestHandleSuite(t *stdtesting.T) { tc.Run(t, &HandleSuite{}) }
func (s *HandleSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	// For testing purposes, don't set the user to run as.
	// Most developers don't have rights to use 'su'.
	s.PatchValue(&machineactions.RunAsUser, "")
}

func (s *HandleSuite) TestInvalidAction(c *tc.C) {
	results, err := machineactions.HandleAction("invalid", nil)
	c.Assert(err, tc.ErrorMatches, "unexpected action invalid")
	c.Assert(results, tc.IsNil)
}

func (s *HandleSuite) TestValidActionInvalidParams(c *tc.C) {
	results, err := machineactions.HandleAction(actions.JujuExecActionName, nil)
	c.Assert(err, tc.ErrorMatches, "invalid action parameters")
	c.Assert(results, tc.IsNil)
}

func (s *HandleSuite) TestTimeoutRun(c *tc.C) {
	params := map[string]interface{}{
		"command": "sleep 100",
		"timeout": float64(1),
	}

	results, err := machineactions.HandleAction(actions.JujuExecActionName, params)
	c.Assert(errors.Cause(err), tc.Equals, exec.ErrCancelled)
	c.Assert(results, tc.IsNil)
}

func (s *HandleSuite) TestSuccessfulRun(c *tc.C) {
	params := map[string]interface{}{
		"command": "echo 1",
		"timeout": float64(0),
	}

	results, err := machineactions.HandleAction(actions.JujuExecActionName, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results["return-code"], tc.Equals, 0)
	c.Assert(strings.TrimRight(results["stdout"].(string), "\r\n"), tc.Equals, "1")
	c.Assert(results["stderr"], tc.Equals, "")
}

func (s *HandleSuite) TestErrorRun(c *tc.C) {
	params := map[string]interface{}{
		"command": "exit 42",
		"timeout": float64(0),
	}

	results, err := machineactions.HandleAction(actions.JujuExecActionName, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results["return-code"], tc.Equals, 42)
	c.Assert(results["stdout"], tc.Equals, "")
	c.Assert(results["stderr"], tc.Equals, "")
}
