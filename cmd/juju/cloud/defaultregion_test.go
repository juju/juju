// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
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
	c.Assert(err, gc.ErrorMatches, `region "foo" for cloud aws not valid, valid regions are .*`)
}

func (s *defaultRegionSuite) TestBadCloudName(c *gc.C) {
	cmd := cloud.NewSetDefaultRegionCommand()
	_, err := testing.RunCommand(c, cmd, "somecloud", "us-west-1")
	c.Assert(err, gc.ErrorMatches, `cloud somecloud not valid`)
}

func (s *defaultRegionSuite) assertSetDefaultRegion(c *gc.C, cmd cmd.Command, store *jujuclienttesting.MemStore, cloud, errStr string) {
	ctx, err := testing.RunCommand(c, cmd, cloud, "us-west-1")
	output := testing.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	if errStr != "" {
		c.Assert(err, gc.ErrorMatches, errStr)
		c.Assert(output, gc.Equals, "")
		c.Assert(store.Credentials[cloud].DefaultRegion, gc.Equals, "")
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Equals, fmt.Sprintf(`Default region in %s set to "us-west-1".`, cloud))
	c.Assert(store.Credentials[cloud].DefaultRegion, gc.Equals, "us-west-1")
}

func (s *defaultRegionSuite) TestSetDefaultRegion(c *gc.C) {
	store := jujuclienttesting.NewMemStore()
	cmd := cloud.NewSetDefaultRegionCommandForTest(store)
	s.assertSetDefaultRegion(c, cmd, store, "aws", "")
}

func (s *defaultRegionSuite) TestSetDefaultRegionBuiltIn(c *gc.C) {
	store := jujuclienttesting.NewMemStore()
	cmd := cloud.NewSetDefaultRegionCommandForTest(store)
	// maas has no regions
	s.assertSetDefaultRegion(c, cmd, store, "maas", `cloud maas has no regions`)
}

func (s *defaultRegionSuite) TestOverwriteDefaultRegion(c *gc.C) {
	store := jujuclienttesting.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{DefaultRegion: "us-east-1"}
	cmd := cloud.NewSetDefaultRegionCommandForTest(store)
	s.assertSetDefaultRegion(c, cmd, store, "aws", "")
}

func (s *defaultRegionSuite) TestCaseInsensitiveRegionSpecification(c *gc.C) {
	store := jujuclienttesting.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{DefaultRegion: "us-east-1"}

	cmd := cloud.NewSetDefaultRegionCommandForTest(store)
	_, err := testing.RunCommand(c, cmd, "aws", "us-WEST-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(store.Credentials["aws"].DefaultRegion, gc.Equals, "us-west-1")
}
