// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/worker/machineactions"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"
)

type HandleSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&HandleSuite{})

func (s *HandleSuite) TestInvalidAction(c *gc.C) {
	results, err := machineactions.HandleAction("invalid", nil)
	c.Assert(err, gc.ErrorMatches, "unexpected action invalid")
	c.Assert(results, gc.IsNil)
}

func (s *HandleSuite) TestValidActionInvalidParams(c *gc.C) {
	results, err := machineactions.HandleAction(actions.JujuRunActionName, nil)
	c.Assert(err, gc.ErrorMatches, "invalid action parameters")
	c.Assert(results, gc.IsNil)
}

func (s *HandleSuite) TestTimeoutRun(c *gc.C) {
	params := map[string]interface{}{
		"command": "sleep 100",
		"timeout": float64(1),
	}

	results, err := machineactions.HandleAction(actions.JujuRunActionName, params)
	c.Assert(errors.Cause(err), gc.Equals, exec.ErrCancelled)
	c.Assert(results, gc.IsNil)
}

func (s *HandleSuite) TestSuccessfulRun(c *gc.C) {
	params := map[string]interface{}{
		"command": "echo 1",
		"timeout": float64(0),
	}

	results, err := machineactions.HandleAction(actions.JujuRunActionName, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results["Code"], gc.Equals, "0")
	c.Assert(strings.TrimRight(results["Stdout"].(string), "\r\n"), gc.Equals, "1")
	c.Assert(results["Stderr"], gc.Equals, "")
}

func (s *HandleSuite) TestErrorRun(c *gc.C) {
	params := map[string]interface{}{
		"command": "exit 42",
		"timeout": float64(0),
	}

	results, err := machineactions.HandleAction(actions.JujuRunActionName, params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results["Code"], gc.Equals, "42")
	c.Assert(results["Stdout"], gc.Equals, "")
	c.Assert(results["Stderr"], gc.Equals, "")
}
