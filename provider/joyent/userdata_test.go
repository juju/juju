// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/cloudinit/cloudinittest"
	"github.com/juju/juju/provider/joyent"
	"github.com/juju/juju/testing"
	"github.com/juju/utils/os"
)

type UserdataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&UserdataSuite{})

func (s *UserdataSuite) TestJoyentUnix(c *gc.C) {
	renderer := joyent.JoyentRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{YAML: []byte("yaml")}

	result, err := renderer.Render(cloudcfg, os.Ubuntu)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cloudcfg.YAML)

	result, err = renderer.Render(cloudcfg, os.CentOS)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cloudcfg.YAML)
}

func (s *UserdataSuite) TestJoyentUnknownOS(c *gc.C) {
	renderer := joyent.JoyentRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{}
	result, err := renderer.Render(cloudcfg, os.Windows)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "Cannot encode userdata for OS: Windows")
}
