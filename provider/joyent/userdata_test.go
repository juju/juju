// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	data := []byte("test")
	result, err := renderer.EncodeUserdata(data, os.Ubuntu)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, data)

	data = []byte("test")
	result, err = renderer.EncodeUserdata(data, os.CentOS)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, data)
}

func (s *UserdataSuite) TestJoyentUnknownOS(c *gc.C) {
	renderer := joyent.JoyentRenderer{}
	result, err := renderer.EncodeUserdata(nil, os.Windows)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "Cannot encode userdata for OS: Windows")
}
