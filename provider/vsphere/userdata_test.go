// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"encoding/base64"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/cloudconfig/cloudinit/cloudinittest"
	"github.com/juju/juju/v2/core/os"
	"github.com/juju/juju/v2/provider/vsphere"
	"github.com/juju/juju/v2/testing"
)

type UserdataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&UserdataSuite{})

func (s *UserdataSuite) TestVsphereUnix(c *gc.C) {
	renderer := vsphere.VsphereRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{YAML: []byte("yaml")}

	result, err := renderer.Render(cloudcfg, os.Ubuntu)
	c.Assert(err, jc.ErrorIsNil)
	expected := base64.StdEncoding.EncodeToString(cloudcfg.YAML)
	c.Assert(string(result), jc.DeepEquals, expected)

	result, err = renderer.Render(cloudcfg, os.CentOS)
	c.Assert(err, jc.ErrorIsNil)
	expected = base64.StdEncoding.EncodeToString(cloudcfg.YAML)
	c.Assert(string(result), jc.DeepEquals, expected)
}

func (s *UserdataSuite) TestVsphereWindows(c *gc.C) {
	renderer := vsphere.VsphereRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{YAML: []byte("yaml")}

	result, err := renderer.Render(cloudcfg, os.Windows)
	c.Assert(err, jc.ErrorIsNil)
	expected := base64.StdEncoding.EncodeToString(cloudcfg.YAML)
	c.Assert(string(result), jc.DeepEquals, expected)
}

func (s *UserdataSuite) TestVsphereUnknownOS(c *gc.C) {
	renderer := vsphere.VsphereRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{}
	result, err := renderer.Render(cloudcfg, os.GenericLinux)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "Cannot encode userdata for OS: GenericLinux")
}
