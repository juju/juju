// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

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

func (*ModelSerializationSuite) modelMap() map[string]interface{} {
	latestTools := version.MustParse("2.0.1")
	configMap := map[string]interface{}{
		"name": "awesome",
		"uuid": "some-uuid",
	}
	return map[string]interface{}{
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
		"services": map[string]interface{}{
			"version": 1,
			"services": []interface{}{
				minimalServiceMap(),
			},
		},
	}
}

func (s *ModelSerializationSuite) TestParsingYAML(c *gc.C) {
	initial := s.modelMap()
	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	model, err := DeserializeModel(bytes)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Owner(), gc.Equals, names.NewUserTag("magic"))
	c.Assert(model.Tag().Id(), gc.Equals, "some-uuid")
	c.Assert(model.Config(), jc.DeepEquals, initial["config"])
	c.Assert(model.LatestToolsVersion(), gc.Equals, version.MustParse("2.0.1"))
	users := model.Users()
	c.Assert(users, gc.HasLen, 1)
	c.Assert(users[0].Name(), gc.Equals, names.NewUserTag("admin@local"))
	machines := model.Machines()
	c.Assert(machines, gc.HasLen, 1)
	c.Assert(machines[0].Id(), gc.Equals, "0")
	services := model.Services()
	c.Assert(services, gc.HasLen, 1)
	c.Assert(services[0].Name(), gc.Equals, "ubuntu")
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
		"services": map[string]interface{}{
			"version":  1,
			"services": []interface{}{},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.LatestToolsVersion(), gc.Equals, version.Zero)
}
