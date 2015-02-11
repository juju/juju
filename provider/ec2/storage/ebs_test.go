// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	ec2storage "github.com/juju/juju/provider/ec2/storage"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type storageSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storageSuite{})

func (*storageSuite) TestValidateConfigNoZone(c *gc.C) {
	p := ec2storage.EBSProvider()
	cfg, err := storage.NewConfig("foo", ec2storage.EBSProviderType, map[string]interface{}{
		"availability-zone": "zone-1",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, gc.ErrorMatches, `"availability-zone" cannot be specified as a pool option as it needs to match the deployed instance`)
}

func (*storageSuite) TestValidateConfigInvalidConfig(c *gc.C) {
	p := ec2storage.EBSProvider()
	cfg, err := storage.NewConfig("foo", ec2storage.EBSProviderType, map[string]interface{}{
		"invalid": "config",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = p.ValidateConfig(cfg)
	c.Assert(err, gc.ErrorMatches, `unknown provider config option "invalid"`)
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
		out := ec2storage.TranslateUserEBSOptions(in)
		c.Assert(out, jc.DeepEquals, map[string]interface{}{
			"volume-type": expected,
			"foo":         "bar",
		})
	}
}
