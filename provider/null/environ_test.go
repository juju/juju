// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/environs/sshstorage"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type environSuite struct {
	testbase.LoggingSuite
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

func (s *environSuite) TestDestroy(c *gc.C) {
	c.Assert(s.env.Destroy(), gc.ErrorMatches, "null provider destruction is not implemented yet")
}

func (s *environSuite) TestLocalStorageConfig(c *gc.C) {
	c.Assert(s.env.StorageDir(), gc.Equals, "/var/lib/juju/storage")
	c.Assert(s.env.cfg.storageListenAddr(), gc.Equals, ":8040")
	c.Assert(s.env.StorageAddr(), gc.Equals, s.env.cfg.storageListenAddr())
	c.Assert(s.env.SharedStorageAddr(), gc.Equals, "")
	c.Assert(s.env.SharedStorageDir(), gc.Equals, "")
}

func (s *environSuite) TestEnvironSupportsCustomSources(c *gc.C) {
	sources, err := tools.GetMetadataSources(s.env)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 2)
	url, err := sources[0].URL("")
	c.Assert(err, gc.IsNil)
	c.Assert(strings.Contains(url, "/tools"), jc.IsTrue)
}

func (s *environSuite) TestEnvironBootstrapStorager(c *gc.C) {
	var sshScript = `
#!/bin/bash --norc
if [ "$*" = "hostname -- bash" ]; then
    # We're executing bash inside ssh. Wait
    # for input to be written before exiting.
    head -n 1 > /dev/null
fi
exec 0<&- # close stdin
echo JUJU-RC: $RC
`[1:]
	bin := c.MkDir()
	ssh := filepath.Join(bin, "ssh")
	err := ioutil.WriteFile(ssh, []byte(sshScript), 0755)
	c.Assert(err, gc.IsNil)
	s.PatchEnvironment("PATH", bin+":"+os.Getenv("PATH"))

	s.PatchEnvironment("RC", "99") // simulate ssh failure
	err = s.env.EnableBootstrapStorage()
	c.Assert(err, gc.ErrorMatches, "exit code 99")
	c.Assert(s.env.Storage(), gc.Not(gc.FitsTypeOf), new(sshstorage.SSHStorage))

	s.PatchEnvironment("RC", "0")
	err = s.env.EnableBootstrapStorage()
	c.Assert(err, gc.IsNil)
	c.Assert(s.env.Storage(), gc.FitsTypeOf, new(sshstorage.SSHStorage))

	// Check idempotency
	err = s.env.EnableBootstrapStorage()
	c.Assert(err, gc.IsNil)
	c.Assert(s.env.Storage(), gc.FitsTypeOf, new(sshstorage.SSHStorage))
}
