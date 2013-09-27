// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/instance"
)

type environSuite struct {
	env *nullEnviron
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	envConfig := getEnvironConfig(c, minimalConfigValues())
	s.env = &nullEnviron{cfg: envConfig}
}

func (s *environSuite) TestSetConfig(c *gc.C) {
	err := s.env.SetConfig(minimalConfig(c))
	c.Assert(err, gc.IsNil)

	testConfig := minimalConfig(c)
	testConfig, err = testConfig.Apply(map[string]interface{}{"bootstrap-host": ""})
	c.Assert(err, gc.IsNil)
	err = s.env.SetConfig(testConfig)
	c.Assert(err, gc.ErrorMatches, "bootstrap-host must be specified")
}

func (s *environSuite) TestInstances(c *gc.C) {
	var ids []instance.Id

	instances, err := s.env.Instances(ids)
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(instances, gc.HasLen, 0)

	ids = append(ids, manual.BootstrapInstanceId)
	instances, err = s.env.Instances(ids)
	c.Assert(err, gc.IsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0], gc.NotNil)

	ids = append(ids, manual.BootstrapInstanceId)
	instances, err = s.env.Instances(ids)
	c.Assert(err, gc.IsNil)
	c.Assert(instances, gc.HasLen, 2)
	c.Assert(instances[0], gc.NotNil)
	c.Assert(instances[1], gc.NotNil)

	ids = append(ids, instance.Id("invalid"))
	instances, err = s.env.Instances(ids)
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(instances, gc.HasLen, 3)
	c.Assert(instances[0], gc.NotNil)
	c.Assert(instances[1], gc.NotNil)
	c.Assert(instances[2], gc.IsNil)

	ids = []instance.Id{instance.Id("invalid")}
	instances, err = s.env.Instances(ids)
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0], gc.IsNil)
}

func (s *environSuite) TestLocalStorageConfig(c *gc.C) {
	c.Assert(s.env.StorageDir(), gc.Equals, "/var/lib/juju/storage")
	c.Assert(s.env.cfg.storageListenAddr(), gc.Equals, ":8040")
	c.Assert(s.env.StorageAddr(), gc.Equals, s.env.cfg.storageListenAddr())
	c.Assert(s.env.SharedStorageAddr(), gc.Equals, "")
	c.Assert(s.env.SharedStorageDir(), gc.Equals, "")
}
