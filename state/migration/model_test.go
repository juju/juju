// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type ModelSerializationSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ModelSerializationSuite{})

func (*ModelSerializationSuite) TestNil(c *gc.C) {
	_, err := importModel(nil)
	c.Check(err, gc.ErrorMatches, "version: expected int, got nothing")
}

func (*ModelSerializationSuite) TestMissingVersion(c *gc.C) {
	_, err := importModel(map[string]interface{}{})
	c.Check(err, gc.ErrorMatches, "version: expected int, got nothing")
}

func (*ModelSerializationSuite) TestNonIntVersion(c *gc.C) {
	_, err := importModel(map[string]interface{}{
		"version": "hello",
	})
	c.Check(err.Error(), gc.Equals, `version: expected int, got string("hello")`)
}

func (*ModelSerializationSuite) TestUnknownVersion(c *gc.C) {
	_, err := importModel(map[string]interface{}{
		"version": 42,
	})
	c.Check(err.Error(), gc.Equals, `version 42 not valid`)
}

func (*ModelSerializationSuite) TestParsing(c *gc.C) {
	latestTools := version.MustParse("2.0.1")
	configMap := map[string]interface{}{
		"name": "awesome",
		"uuid": "some-uuid",
	}
	model, err := importModel(map[string]interface{}{
		"version":      1,
		"owner":        "magic",
		"config":       configMap,
		"latest-tools": latestTools.String(),
		"users": map[string]interface{}{
			"version": 1,
			"users": []interface{}{
				map[string]interface{}{
					"name":         "admin@local",
					"created-by":   "admin@local",
					"date-created": time.Date(2015, 10, 9, 12, 34, 56, 0, time.UTC),
				},
			},
		},
		"machines": map[string]interface{}{
			"version": 1,
			"machines": []interface{}{
				minimalMachineMap("0"),
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Owner_, gc.Equals, "magic")
	c.Assert(model.Tag().Id(), gc.Equals, "some-uuid")
	c.Assert(model.Config_, jc.DeepEquals, configMap)
	c.Assert(model.LatestToolsVersion(), gc.Equals, latestTools)
	c.Assert(model.Users_.Users_, gc.HasLen, 1)
	c.Assert(model.Users_.Users_[0].Name_, gc.Equals, "admin@local")
	c.Assert(model.Machines_.Machines_, gc.HasLen, 1)
	c.Assert(model.Machines_.Machines_[0].Id_, gc.Equals, "0")
}

func (*ModelSerializationSuite) TestParsingOptionals(c *gc.C) {
	configMap := map[string]interface{}{
		"name": "awesome",
		"uuid": "some-uuid",
	}
	model, err := importModel(map[string]interface{}{
		"version": 1,
		"owner":   "magic",
		"config":  configMap,
		"users": map[string]interface{}{
			"version": 1,
			"users":   []interface{}{},
		},
		"machines": map[string]interface{}{
			"version":  1,
			"machines": []interface{}{},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.LatestToolsVersion(), gc.Equals, version.Zero)
}
