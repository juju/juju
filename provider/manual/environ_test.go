// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"os"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	coretesting "github.com/juju/juju/testing"
)

type environSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	env *manualEnviron
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
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
		c.Assert(stdin, jc.DeepEquals, `
set -x
touch '/var/lib/juju/uninstall-agent'
pkill -6 jujud && exit
stop juju-db
rm -f /etc/init/juju*
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
	cons := constraints.MustParse("arch=amd64 instance-type=foo tags=bar cpu-power=10 cpu-cores=2 mem=1G virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power", "instance-type", "tags", "virt-type"})
}

type bootstrapSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	env *manualEnviron
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	env, err := manualProvider{}.Open(MinimalConfig(c))
	c.Assert(err, jc.ErrorIsNil)
	s.env = env.(*manualEnviron)
}

type controllerInstancesSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	env *manualEnviron
}

var _ = gc.Suite(&controllerInstancesSuite{})

func (s *controllerInstancesSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	env, err := manualProvider{}.Open(MinimalConfig(c))
	c.Assert(err, jc.ErrorIsNil)
	s.env = env.(*manualEnviron)
}

func (s *controllerInstancesSuite) TestControllerInstances(c *gc.C) {
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
		expectedErr: "model is not bootstrapped",
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
		instances, err := s.env.ControllerInstances()
		if test.expectedErr == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(instances, gc.DeepEquals, []instance.Id{BootstrapInstanceId})
		} else {
			c.Assert(err, gc.ErrorMatches, test.expectedErr)
			c.Assert(instances, gc.HasLen, 0)
		}
	}
}

func (s *controllerInstancesSuite) TestControllerInstancesStderr(c *gc.C) {
	// Stderr should not affect the behaviour of ControllerInstances.
	testing.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 0")
	_, err := s.env.ControllerInstances()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *controllerInstancesSuite) TestControllerInstancesError(c *gc.C) {
	// If the ssh execution fails, its stderr will be captured in the error message.
	testing.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 1")
	_, err := s.env.ControllerInstances()
	c.Assert(err, gc.ErrorMatches, "abc: .*")
}

func (s *controllerInstancesSuite) TestControllerInstancesInternal(c *gc.C) {
	// Patch os.Args so it appears that we're running in "jujud".
	s.PatchValue(&os.Args, []string{"/some/where/containing/jujud", "whatever"})
	// Patch the ssh executable so that it would cause an error if we
	// were to call it.
	testing.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 1")
	instances, err := s.env.ControllerInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.DeepEquals, []instance.Id{BootstrapInstanceId})
}
