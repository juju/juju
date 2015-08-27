// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	coretesting "github.com/juju/juju/testing"
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
	c.Assert(err, jc.ErrorIsNil)
	s.env = env.(*manualEnviron)
}

func (s *environSuite) TestSetConfig(c *gc.C) {
	err := s.env.SetConfig(MinimalConfig(c))
	c.Assert(err, jc.ErrorIsNil)

	testConfig := MinimalConfig(c)
	testConfig, err = testConfig.Apply(map[string]interface{}{"bootstrap-host": ""})
	c.Assert(err, jc.ErrorIsNil)
	err = s.env.SetConfig(testConfig)
	c.Assert(err, gc.ErrorMatches, "bootstrap-host must be specified")
}

func (s *environSuite) TestInstances(c *gc.C) {
	var ids []instance.Id

	instances, err := s.env.Instances(ids)
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(instances, gc.HasLen, 0)

	ids = append(ids, BootstrapInstanceId)
	instances, err = s.env.Instances(ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0], gc.NotNil)

	ids = append(ids, BootstrapInstanceId)
	instances, err = s.env.Instances(ids)
	c.Assert(err, jc.ErrorIsNil)
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
	}
	for i, t := range tests {
		c.Logf("test %d: %v", i, t)
		resultStderr, resultErr = t.stderr, t.err
		err := s.env.Destroy()
		if t.match == "" {
			c.Assert(err, jc.ErrorIsNil)
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

func (s *environSuite) TestSupportedArchitectures(c *gc.C) {
	arches, err := s.env.SupportedArchitectures()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(arches, gc.DeepEquals, arch.AllSupportedArches)
}

func (s *environSuite) TestSupportsNetworking(c *gc.C) {
	_, ok := environs.SupportsNetworking(s.env)
	c.Assert(ok, jc.IsFalse)
}

func (s *environSuite) TestConstraintsValidator(c *gc.C) {
	validator, err := s.env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 instance-type=foo tags=bar cpu-power=10 cpu-cores=2 mem=1G")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power", "instance-type", "tags"})
}

type bootstrapSuite struct {
	coretesting.FakeJujuHomeSuite
	env *manualEnviron
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)

	// ensure use-sshstorage=true to mimic what happens
	// in the real client: the environment is Prepared,
	// at which point use-sshstorage=true.
	cfg := MinimalConfig(c)
	cfg, err := cfg.Apply(map[string]interface{}{
		"use-sshstorage": true,
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := manualProvider{}.Open(cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.env = env.(*manualEnviron)
}

func (s *bootstrapSuite) TestBootstrapClearsUseSSHStorage(c *gc.C) {
	s.PatchValue(&manualDetectSeriesAndHardwareCharacteristics, func(string) (instance.HardwareCharacteristics, string, error) {
		arch := arch.HostArch()
		return instance.HardwareCharacteristics{Arch: &arch}, "precise", nil
	})
	s.PatchValue(&manualCheckProvisioned, func(string) (bool, error) {
		return false, nil
	})

	// use-sshstorage is initially true.
	cfg := s.env.Config()
	c.Assert(cfg.UnknownAttrs()["use-sshstorage"], jc.IsTrue)

	_, _, _, err := s.env.Bootstrap(envtesting.BootstrapContext(c), environs.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)

	// Bootstrap must set use-sshstorage to false within the environment.
	cfg = s.env.Config()
	c.Assert(cfg.UnknownAttrs()["use-sshstorage"], jc.IsFalse)
}

type stateServerInstancesSuite struct {
	coretesting.FakeJujuHomeSuite
	env *manualEnviron
}

var _ = gc.Suite(&stateServerInstancesSuite{})

func (s *stateServerInstancesSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)

	// ensure use-sshstorage=true, or bootstrap-host
	// verification won't happen in StateServerInstances.
	cfg := MinimalConfig(c)
	cfg, err := cfg.Apply(map[string]interface{}{
		"use-sshstorage": true,
	})
	c.Assert(err, jc.ErrorIsNil)

	env, err := manualProvider{}.Open(cfg)
	c.Assert(err, jc.ErrorIsNil)
	s.env = env.(*manualEnviron)
}

func (s *stateServerInstancesSuite) TestStateServerInstances(c *gc.C) {
	var outputResult string
	var errResult error
	runSSHCommandTesting := func(host string, command []string, stdin string) (string, error) {
		return outputResult, errResult
	}
	s.PatchValue(&runSSHCommand, runSSHCommandTesting)

	type test struct {
		output      string
		err         error
		expectedErr string
	}
	tests := []test{{
		output: "",
	}, {
		output:      "no-agent-dir",
		expectedErr: "environment is not bootstrapped",
	}, {
		output:      "woo",
		expectedErr: `unexpected output: "woo"`,
	}, {
		err:         errors.New("an error"),
		expectedErr: "an error",
	}}

	for i, test := range tests {
		c.Logf("test %d", i)
		outputResult = test.output
		errResult = test.err
		instances, err := s.env.StateServerInstances()
		if test.expectedErr == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(instances, gc.DeepEquals, []instance.Id{BootstrapInstanceId})
		} else {
			c.Assert(err, gc.ErrorMatches, test.expectedErr)
			c.Assert(instances, gc.HasLen, 0)
		}
	}
}

func (s *stateServerInstancesSuite) TestStateServerInstancesStderr(c *gc.C) {
	// Stderr should not affect the behaviour of StateServerInstances.
	testing.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 0")
	_, err := s.env.StateServerInstances()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateServerInstancesSuite) TestStateServerInstancesError(c *gc.C) {
	// If the ssh execution fails, its stderr will be captured in the error message.
	testing.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 1")
	_, err := s.env.StateServerInstances()
	c.Assert(err, gc.ErrorMatches, "abc: .*")
}

func (s *stateServerInstancesSuite) TestStateServerInstancesInternal(c *gc.C) {
	// If use-sshstorage=false, then we're on the bootstrap host;
	// verification is elided.
	env, err := manualProvider{}.Open(MinimalConfig(c))
	c.Assert(err, jc.ErrorIsNil)

	testing.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 1")
	instances, err := env.StateServerInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.DeepEquals, []instance.Id{BootstrapInstanceId})
}
