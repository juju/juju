// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/providerinit/renderers"
	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type UserdataSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&UserdataSuite{})

func (s *UserdataSuite) TestAmazonUnix(c *gc.C) {
	renderer := ec2.AmazonRenderer{}
	data := []byte("test")
	result, err := renderer.EncodeUserdata(data, version.Ubuntu)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, utils.Gzip(data))

	data = []byte("test")
	result, err = renderer.EncodeUserdata(data, version.CentOS)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, utils.Gzip(data))
}

func (s *UserdataSuite) TestAmazonWindows(c *gc.C) {
	renderer := ec2.AmazonRenderer{}
	data := []byte("test")
	result, err := renderer.EncodeUserdata(data, version.Windows)
	c.Assert(err, jc.ErrorIsNil)
	expected := []byte(`<powershell>` +
		string(renderers.WinEmbedInScript(data)) +
		`</powershell>`)
	c.Assert(result, jc.DeepEquals, expected)
}

func (s *UserdataSuite) TestAmazonUnknownOS(c *gc.C) {
	renderer := ec2.AmazonRenderer{}
	result, err := renderer.EncodeUserdata(nil, version.Arch)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "Cannot encode userdata for OS: Arch")
}
