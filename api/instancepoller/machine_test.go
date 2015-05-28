// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/instancepoller"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type MachineSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&MachineSuite{})

func (s *MachineSuite) TestNonFacadeMethods(c *gc.C) {
	nopCaller := apitesting.APICallerFunc(
		func(_ string, _ int, _, _ string, _, _ interface{}) error {
			c.Fatalf("facade call was not expected")
			return errors.New("boom")
		},
	)
	tag := names.NewMachineTag("42")
	machine := instancepoller.NewMachine(nopCaller, tag, params.Dying)

	c.Assert(machine.Id(), gc.Equals, "42")
	c.Assert(machine.Tag(), jc.DeepEquals, tag)
	c.Assert(machine.String(), gc.Equals, "42")
	c.Assert(machine.Life(), gc.Equals, params.Dying)
}
