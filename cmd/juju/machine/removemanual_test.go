// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type RemoveManualMachineSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&RemoveManualMachineSuite{})

func (s *RemoveManualMachineSuite) run(c *gc.C, args ...string) (*cmd.Context, error) {
	remove, _ := machine.NewRemoveManualCommandForTest()
	return cmdtesting.RunCommand(c, remove, args...)
}

func (s *RemoveManualMachineSuite) TestInit(c *gc.C) {
	for i, test := range []struct {
		args        []string
		placement   *instance.Placement
		errorString string
	}{
		{
			errorString: "wrong number of arguments, expected 1",
		},
		{
			args:      []string{"ssh:user@10.10.0.3"},
			placement: &instance.Placement{Scope: "ssh", Directive: "user@10.10.0.3"},
		},
		{
			args:      []string{"winrm:user@10.10.0.3"},
			placement: &instance.Placement{Scope: "winrm", Directive: "user@10.10.0.3"},
		},
		{
			args:      []string{"winrm:10.10.0.3"},
			placement: &instance.Placement{Scope: "winrm", Directive: "10.10.0.3"},
		},
		{
			args:        []string{"1"},
			errorString: "remove-manual-machine expects user@host argument. Instead please use remove-machine 1",
		},
		{
			args:        []string{"lxd"},
			errorString: "invalid placement directive \"lxd\"",
		},
	} {
		c.Logf("test %d", i)
		wrappedCommand, removeCmd := machine.NewRemoveManualCommandForTest()
		err := cmdtesting.InitCommand(wrappedCommand, test.args)
		if test.errorString == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(removeCmd.Placement, jc.DeepEquals, test.placement)
		} else {
			c.Check(err, gc.ErrorMatches, test.errorString)
		}
	}
}
