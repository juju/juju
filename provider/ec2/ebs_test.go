// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/ec2"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type storageSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageSuite{})

func (*storageSuite) TestValidateConfigInvalidConfig(c *gc.C) {
	p := ec2.EBSProvider()
	cfg, err := storage.NewConfig("foo", ec2.EBS_ProviderType, map[string]interface{}{
		"invalid": "config",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, gc.ErrorMatches, `unknown provider config option "invalid"`)
}

func (s *storageSuite) TestSupports(c *gc.C) {
	p := ec2.EBSProvider()
	c.Assert(p.Supports(storage.StorageKindBlock), jc.IsTrue)
	c.Assert(p.Supports(storage.StorageKindFilesystem), jc.IsFalse)
}

func (*storageSuite) TestTranslateUserEBSOptions(c *gc.C) {
	for _, vType := range []string{"magnetic", "ssd", "provisioned-iops"} {
		in := map[string]interface{}{
			"volume-type": vType,
			"foo":         "bar",
		}
		var expected string
		switch vType {
		case "magnetic":
			expected = "standard"
		case "ssd":
			expected = "gp2"
		case "provisioned-iops":
			expected = "io1"
		}
		out := ec2.TranslateUserEBSOptions(in)
		c.Assert(out, jc.DeepEquals, map[string]interface{}{
			"volume-type": expected,
			"foo":         "bar",
		})
	}
}
