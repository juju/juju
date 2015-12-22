// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/os"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/cloudinit/cloudinittest"
	"github.com/juju/juju/cloudconfig/providerinit/renderers"
	"github.com/juju/juju/provider/openstack"
	"github.com/juju/juju/testing"
)

type UserdataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&UserdataSuite{})

func (s *UserdataSuite) TestOpenstackUnix(c *gc.C) {
	renderer := openstack.OpenstackRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{YAML: []byte("yaml")}

	result, err := renderer.Render(cloudcfg, os.Ubuntu)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, utils.Gzip(cloudcfg.YAML))

	result, err = renderer.Render(cloudcfg, os.CentOS)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, utils.Gzip(cloudcfg.YAML))
}

func (s *UserdataSuite) TestOpenstackWindows(c *gc.C) {
	renderer := openstack.OpenstackRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{YAML: []byte("yaml")}

	result, err := renderer.Render(cloudcfg, os.Windows)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, renderers.WinEmbedInScript(cloudcfg.YAML))
}

func (s *UserdataSuite) TestOpenstackUnknownOS(c *gc.C) {
	renderer := openstack.OpenstackRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{}
	result, err := renderer.Render(cloudcfg, os.Arch)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "Cannot encode userdata for OS: Arch")
}
