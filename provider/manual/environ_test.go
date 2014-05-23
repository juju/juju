// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/arch"
	coretesting "launchpad.net/juju-core/testing"
)

type environSuite struct {
	coretesting.FakeJujuHomeSuite
	env *manualEnviron
}

type dummyStorage struct {
	storage.Storage
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	env, err := manualProvider{}.Open(MinimalConfig(c))
	c.Assert(err, gc.IsNil)
	s.env = env.(*manualEnviron)
}

func (s *environSuite) TestSetConfig(c *gc.C) {
	err := s.env.SetConfig(MinimalConfig(c))
	c.Assert(err, gc.IsNil)

	testConfig := MinimalConfig(c)
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
	var resultStderr string
	var resultErr error
	runSSHCommandTesting := func(host string, command []string, stdin string) (string, error) {
		c.Assert(host, gc.Equals, "ubuntu@hostname")
		c.Assert(command, gc.DeepEquals, []string{"sudo", "/bin/bash"})
		c.Assert(stdin, gc.DeepEquals, `
set -x
pkill -6 jujud && exit
stop juju-db
rm -f /etc/init/juju*
rm -f /etc/rsyslog.d/*juju*
rm -fr '/var/lib/juju' '/var/log/juju'
exit 0
`)
		return resultStderr, resultErr
	}
	s.PatchValue(&runSSHCommand, runSSHCommandTesting)
	type test struct {
		stderr string
		err    error
		match  string
	}
	tests := []test{
		{"", nil, ""},
		{"abc", nil, ""},
		{"", errors.New("oh noes"), "oh noes"},
		{"123", errors.New("abc"), "abc \\(123\\)"},
	}
	for i, t := range tests {
		c.Logf("test %d: %v", i, t)
		resultStderr, resultErr = t.stderr, t.err
		err := s.env.Destroy()
		if t.match == "" {
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, t.match)
		}
	}
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

func (s *environSuite) TestSupportedArchitectures(c *gc.C) {
	arches, err := s.env.SupportedArchitectures()
	c.Assert(err, gc.IsNil)
	c.Assert(arches, gc.DeepEquals, arch.AllSupportedArches)
}

func (s *environSuite) TestSupportNetworks(c *gc.C) {
	c.Assert(s.env.SupportNetworks(), jc.IsFalse)
}

func (s *environSuite) TestConstraintsValidator(c *gc.C) {
	validator, err := s.env.ConstraintsValidator()
	c.Assert(err, gc.IsNil)
	cons := constraints.MustParse("arch=amd64 instance-type=foo tags=bar cpu-power=10 cpu-cores=2 mem=1G")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, gc.IsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power", "instance-type", "tags"})
}
