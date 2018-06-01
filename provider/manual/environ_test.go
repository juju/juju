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
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/instance"
	coretesting "github.com/juju/juju/testing"
)

type baseEnvironSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	env *manualEnviron

	callCtx context.ProviderCallContext
}

func (s *baseEnvironSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	env, err := ManualProvider{}.Open(environs.OpenParams{
		Cloud:  CloudSpec(),
		Config: MinimalConfig(c),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.env = env.(*manualEnviron)
	s.callCtx = context.NewCloudCallContext()
}

type environSuite struct {
	baseEnvironSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestInstances(c *gc.C) {
	var ids []instance.Id

	instances, err := s.env.Instances(s.callCtx, ids)
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(instances, gc.HasLen, 0)

	ids = append(ids, BootstrapInstanceId)
	instances, err = s.env.Instances(s.callCtx, ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0], gc.NotNil)

	ids = append(ids, BootstrapInstanceId)
	instances, err = s.env.Instances(s.callCtx, ids)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 2)
	c.Assert(instances[0], gc.NotNil)
	c.Assert(instances[1], gc.NotNil)

	ids = append(ids, instance.Id("invalid"))
	instances, err = s.env.Instances(s.callCtx, ids)
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(instances, gc.HasLen, 3)
	c.Assert(instances[0], gc.NotNil)
	c.Assert(instances[1], gc.NotNil)
	c.Assert(instances[2], gc.IsNil)

	ids = []instance.Id{instance.Id("invalid")}
	instances, err = s.env.Instances(s.callCtx, ids)
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0], gc.IsNil)
}

func (s *environSuite) TestDestroyController(c *gc.C) {
	var resultStdout string
	var resultErr error
	runSSHCommandTesting := func(host string, command []string, stdin string) (string, string, error) {
		c.Assert(host, gc.Equals, "ubuntu@hostname")
		c.Assert(command, gc.DeepEquals, []string{"sudo", "/bin/bash"})
		c.Assert(stdin, gc.Equals, `
# Signal the jujud process to stop, then check it has done so before cleaning-up
# after it.
set -x
touch '/var/lib/juju/uninstall-agent'

stopped=0
function wait_for_jujud {
    for i in {1..30}; do
        if pgrep jujud > /dev/null ; then
            sleep 1
        else
            echo jujud stopped
            stopped=1
            logger --id jujud stopped on attempt $i
            break
        fi
    done
}

# There might be no jujud at all (for example, after a failed deployment) so
# don't require pkill to succeed before looking for a jujud process.
# SIGABRT not SIGTERM, as abort lets the worker know it should uninstall itself,
# rather than terminate normally.
pkill -SIGABRT jujud
wait_for_jujud

[[ $stopped -ne 1 ]] && {
    # If jujud didn't stop nicely, we kill it hard here.
    pkill -SIGKILL jujud && wait_for_jujud
}
[[ $stopped -ne 1 ]] && {
    echo jujud removal failed
    logger --id $(ps -o pid,cmd,state -p $(pgrep jujud) | awk 'NR != 1 {printf("Process %d (%s) has state %s\n", $1, $2, $3)}')
    exit 1
}
service juju-db stop && logger --id stopped juju-db
apt-get -y purge juju-mongo*
apt-get -y autoremove
rm -f /etc/init/juju*
rm -f /etc/systemd/system{,/multi-user.target.wants}/juju*
rm -fr '/var/lib/juju' '/var/log/juju'
exit 0
`)
		return resultStdout, "", resultErr
	}
	s.PatchValue(&runSSHCommand, runSSHCommandTesting)
	type test struct {
		stdout string
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
		resultStdout, resultErr = t.stdout, t.err
		err := s.env.DestroyController(s.callCtx, "controller-uuid")
		if t.match == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, t.match)
		}
	}
}

func (s *environSuite) TestSupportsNetworking(c *gc.C) {
	_, ok := environs.SupportsNetworking(s.env)
	c.Assert(ok, jc.IsFalse)
}

func (s *environSuite) TestConstraintsValidator(c *gc.C) {
	s.PatchValue(&sshprovisioner.DetectSeriesAndHardwareCharacteristics,
		func(string) (instance.HardwareCharacteristics, string, error) {
			amd64 := "amd64"
			return instance.HardwareCharacteristics{
				Arch: &amd64,
			}, "", nil
		},
	)

	validator, err := s.env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 instance-type=foo tags=bar cpu-power=10 cores=2 mem=1G virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power", "instance-type", "tags", "virt-type"})
}

func (s *environSuite) TestConstraintsValidatorInsideController(c *gc.C) {
	// Patch os.Args so it appears that we're running in "jujud", and then
	// patch the host arch so it looks like we're running arm64.
	s.PatchValue(&os.Args, []string{"/some/where/containing/jujud", "whatever"})
	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })

	validator, err := s.env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=arm64")
	_, err = validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
}

type controllerInstancesSuite struct {
	baseEnvironSuite
}

var _ = gc.Suite(&controllerInstancesSuite{})

func (s *controllerInstancesSuite) TestControllerInstances(c *gc.C) {
	var outputResult string
	var errResult error
	runSSHCommandTesting := func(host string, command []string, stdin string) (string, string, error) {
		return outputResult, "", errResult
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
		instances, err := s.env.ControllerInstances(s.callCtx, "not-used")
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
	_, err := s.env.ControllerInstances(s.callCtx, "not-used")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *controllerInstancesSuite) TestControllerInstancesError(c *gc.C) {
	// If the ssh execution fails, its stderr will be captured in the error message.
	testing.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 1")
	_, err := s.env.ControllerInstances(s.callCtx, "not-used")
	c.Assert(err, gc.ErrorMatches, "abc: .*")
}

func (s *controllerInstancesSuite) TestControllerInstancesInternal(c *gc.C) {
	// Patch os.Args so it appears that we're running in "jujud".
	s.PatchValue(&os.Args, []string{"/some/where/containing/jujud", "whatever"})
	// Patch the ssh executable so that it would cause an error if we
	// were to call it.
	testing.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 1")
	instances, err := s.env.ControllerInstances(s.callCtx, "not-used")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances, gc.DeepEquals, []instance.Id{BootstrapInstanceId})
}
