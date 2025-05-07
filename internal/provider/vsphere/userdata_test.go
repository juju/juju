// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"encoding/base64"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/internal/cloudconfig/cloudinit/cloudinittest"
	"github.com/juju/juju/internal/provider/vsphere"
	"github.com/juju/juju/internal/testing"
)

type UserdataSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&UserdataSuite{})

func (s *UserdataSuite) TestVsphereUnix(c *tc.C) {
	renderer := vsphere.VsphereRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{YAML: []byte("yaml")}

	result, err := renderer.Render(cloudcfg, ostype.Ubuntu)
	c.Assert(err, jc.ErrorIsNil)
	expected := base64.StdEncoding.EncodeToString(cloudcfg.YAML)
	c.Assert(string(result), jc.DeepEquals, expected)
}

func (s *UserdataSuite) TestVsphereUnknownOS(c *tc.C) {
	renderer := vsphere.VsphereRenderer{}
	cloudcfg := &cloudinittest.CloudConfig{}
	result, err := renderer.Render(cloudcfg, ostype.GenericLinux)
	c.Assert(result, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "Cannot encode userdata for OS: GenericLinux")
}
