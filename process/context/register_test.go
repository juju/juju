// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type registerSuite struct {
	baseSuite
}

var _ = gc.Suite(&registerSuite{})

func (r *registerSuite) TestRun(c *gc.C) {
}

func (r *registerSuite) TestInit(c *gc.C) {
}

func (r *registerSuite) TestInfo(c *gc.C) {
}
