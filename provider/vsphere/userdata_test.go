// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"encoding/base64"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/vsphere"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type UserdataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&UserdataSuite{})

func (s *UserdataSuite) TestVsphereUnix(c *gc.C) {
	renderer := vsphere.VsphereRenderer{}
	data := []byte("test")
	result, err := renderer.EncodeUserdata(data, version.Ubuntu)
	c.Assert(err, jc.ErrorIsNil)
	expected := base64.StdEncoding.EncodeToString(data)
	c.Assert(string(result), jc.DeepEquals, expected)

	data = []byte("test")
	result, err = renderer.EncodeUserdata(data, version.CentOS)
	c.Assert(err, jc.ErrorIsNil)
	expected = base64.StdEncoding.EncodeToString(data)
	c.Assert(string(result), jc.DeepEquals, expected)
}

func (s *UserdataSuite) TestVsphereUnknownOS(c *gc.C) {
	renderer := vsphere.VsphereRenderer{}
	result, err := renderer.EncodeUserdata(nil, version.Windows)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "Cannot encode userdata for OS: Windows")
}
