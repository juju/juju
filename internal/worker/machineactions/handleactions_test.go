// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/exec"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/internal/worker/machineactions"
)

type HandleSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&HandleSuite{})

func (s *HandleSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	// For testing purposes, don't set the user to run as.
	// Most developers don't have rights to use 'su'.
	s.PatchValue(&machineactions.RunAsUser, "")
}

func (s *HandleSuite) TestInvalidAction(c *gc.C) {
	results, err := machineactions.HandleAction("invalid", nil)
	c.Assert(err, gc.ErrorMatches, "unexpected action invalid")
	c.Assert(results, gc.IsNil)
}

func (s *HandleSuite) TestValidActionInvalidParams(c *gc.C) {
	results, err := machineactions.HandleAction(actions.JujuExecActionName, nil)
	c.Assert(err, gc.ErrorMatches, "invalid action parameters")
	c.Assert(results, gc.IsNil)
}

func (s *HandleSuite) TestTimeoutRun(c *gc.C) {
	params := map[string]interface{}{
		"command": "sleep 100",
		"timeout": float64(1),
	}

	results, err := machineactions.HandleAction(actions.JujuExecActionName, params)
	c.Assert(errors.Cause(err), gc.Equals, exec.ErrCancelled)
	c.Assert(results, gc.IsNil)
}

func (s *HandleSuite) TestSuccessfulRun(c *gc.C) {
	params := map[string]interface{}{
		"command": "echo 1",
		"timeout": float64(0),
	}

	results, err := machineactions.HandleAction(actions.JujuExecActionName, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results["return-code"], gc.Equals, 0)
	c.Assert(strings.TrimRight(results["stdout"].(string), "\r\n"), gc.Equals, "1")
	c.Assert(results["stderr"], gc.Equals, "")
}

func (s *HandleSuite) TestErrorRun(c *gc.C) {
	params := map[string]interface{}{
		"command": "exit 42",
		"timeout": float64(0),
	}

	results, err := machineactions.HandleAction(actions.JujuExecActionName, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results["return-code"], gc.Equals, 42)
	c.Assert(results["stdout"], gc.Equals, "")
	c.Assert(results["stderr"], gc.Equals, "")
}
