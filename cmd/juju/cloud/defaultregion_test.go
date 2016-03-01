// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/cmd"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type defaultRegionSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&defaultRegionSuite{})

func (s *defaultRegionSuite) SetUpTest(c *gc.C) {
	origHome := osenv.SetJujuXDGDataHome(c.MkDir())
	s.AddCleanup(func(*gc.C) { osenv.SetJujuXDGDataHome(origHome) })
}

func (s *defaultRegionSuite) TestBadArgs(c *gc.C) {
	cmd := cloud.NewSetDefaultRegionCommand()
	_, err := testing.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, "Usage: juju set-default-region <cloud-name> <region>")
	_, err = testing.RunCommand(c, cmd, "cloud", "region", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *defaultRegionSuite) TestBadRegion(c *gc.C) {
	cmd := cloud.NewSetDefaultRegionCommand()
	_, err := testing.RunCommand(c, cmd, "aws", "foo")
	c.Assert(err, gc.ErrorMatches, `region "foo" for cloud aws not valid`)
}

func (s *defaultRegionSuite) TestBadCloudName(c *gc.C) {
	cmd := cloud.NewSetDefaultRegionCommand()
	_, err := testing.RunCommand(c, cmd, "somecloud", "us-west-1")
	c.Assert(err, gc.ErrorMatches, `cloud somecloud not found`)
}

func (s *defaultRegionSuite) assertSetDefaultRegion(c *gc.C, cmd cmd.Command, store *jujuclienttesting.MemStore) {
	ctx, err := testing.RunCommand(c, cmd, "aws", "us-west-1")
	c.Assert(err, jc.ErrorIsNil)
	output := testing.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, `Default region in aws set to "us-west-1".`)
	c.Assert(store.Credentials["aws"].DefaultRegion, gc.Equals, "us-west-1")
}

func (s *defaultRegionSuite) TestSetDefaultRegion(c *gc.C) {
	store := jujuclienttesting.NewMemStore()
	cmd := cloud.NewsetDefaultRegionCommandForTest(store)
	s.assertSetDefaultRegion(c, cmd, store)
}

func (s *defaultRegionSuite) TestOverwriteDefaultRegion(c *gc.C) {
	store := jujuclienttesting.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{DefaultRegion: "us-east-1"}
	cmd := cloud.NewsetDefaultRegionCommandForTest(store)
	s.assertSetDefaultRegion(c, cmd, store)
}
