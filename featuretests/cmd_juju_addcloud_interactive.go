// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"io/ioutil"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type cmdAddCloudInteractiveSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

func (*cmdAddCloudInteractiveSuite) TestRunCmd_CloudsYamlWritten(c *gc.C) {
	ctx := testing.Context(c)
	ctx.Stdin = strings.NewReader("" +
		/* Select cloud type: */ "vsphere\n" +
		/* Enter a name for the cloud: */ "mvs\n" +
		/* Enter the controller's hostname or IP address: */ "192.168.1.6\n" +
		/* Enter region name: */ "foo\n" +
		/* Enter another region? (Y/n): */ "n\n",
	)

	jujuCmd := commands.NewJujuCommand(ctx)
	c.Assert(testing.InitCommand(jujuCmd, []string{"add-cloud"}), jc.ErrorIsNil)
	err := jujuCmd.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)

	data, err := ioutil.ReadFile(osenv.JujuXDGDataHomePath("clouds.yaml"))
	c.Assert(err, jc.ErrorIsNil)

	c.Check(string(data), gc.Equals, ""+
		"clouds:\n"+
		"  mvs:\n"+
		"    type: vsphere\n"+
		"    auth-types: [userpass]\n"+
		"    endpoint: 192.168.1.6\n"+
		"    regions:\n"+
		"      foo:\n"+
		"        endpoint: 192.168.1.6\n",
	)
}
