// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma_test

import (
	"encoding/base64"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/os"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/cloudsigma"
	"github.com/juju/juju/testing"
)

type UserdataSuite struct{ testing.BaseSuite }

var _ = gc.Suite(&UserdataSuite{})

func (s *UserdataSuite) TestCloudSigmaUnix(c *gc.C) {
	renderer := cloudsigma.CloudSigmaRenderer{}
	data := []byte("test")
	result, err := renderer.EncodeUserdata(data, os.Ubuntu)
	c.Assert(err, jc.ErrorIsNil)
	expected := base64.StdEncoding.EncodeToString(data)
	c.Assert(string(result), jc.DeepEquals, expected)

	data = []byte("test")
	result, err = renderer.EncodeUserdata(data, os.CentOS)
	c.Assert(err, jc.ErrorIsNil)
	expected = base64.StdEncoding.EncodeToString(data)
	c.Assert(string(result), jc.DeepEquals, expected)
}

func (s *UserdataSuite) TestCloudSigmaUnknownOS(c *gc.C) {
	renderer := cloudsigma.CloudSigmaRenderer{}
	result, err := renderer.EncodeUserdata(nil, os.Windows)
	c.Assert(result, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "Cannot encode userdata for OS: Windows")
}
