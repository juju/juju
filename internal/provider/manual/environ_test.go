// Copyright 2012 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"os"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type baseEnvironSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	env *manualEnviron
}

func (s *baseEnvironSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	env, err := ManualProvider{}.Open(c.Context(), environs.OpenParams{
		Cloud:  CloudSpec(),
		Config: MinimalConfig(c),
	}, environs.NoopCredentialInvalidator())
	c.Assert(err, tc.ErrorIsNil)
	s.env = env.(*manualEnviron)
}

type environSuite struct {
	baseEnvironSuite
}

func TestEnvironSuite(t *stdtesting.T) {
	tc.Run(t, &environSuite{})
}

func (s *environSuite) TestInstances(c *tc.C) {
	var ids []instance.Id

	instances, err := s.env.Instances(c.Context(), ids)
	c.Assert(err, tc.Equals, environs.ErrNoInstances)
	c.Assert(instances, tc.HasLen, 0)

	ids = append(ids, BootstrapInstanceId)
	instances, err = s.env.Instances(c.Context(), ids)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances, tc.HasLen, 1)
	c.Assert(instances[0], tc.NotNil)

	ids = append(ids, BootstrapInstanceId)
	instances, err = s.env.Instances(c.Context(), ids)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances, tc.HasLen, 2)
	c.Assert(instances[0], tc.NotNil)
	c.Assert(instances[1], tc.NotNil)

	ids = append(ids, instance.Id("invalid"))
	instances, err = s.env.Instances(c.Context(), ids)
	c.Assert(err, tc.Equals, environs.ErrPartialInstances)
	c.Assert(instances, tc.HasLen, 3)
	c.Assert(instances[0], tc.NotNil)
	c.Assert(instances[1], tc.NotNil)
	c.Assert(instances[2], tc.IsNil)

	ids = []instance.Id{instance.Id("invalid")}
	instances, err = s.env.Instances(c.Context(), ids)
	c.Assert(err, tc.Equals, environs.ErrNoInstances)
	c.Assert(instances, tc.HasLen, 1)
	c.Assert(instances[0], tc.IsNil)
}

func (s *environSuite) TestDestroyController(c *tc.C) {
	var resultStdout string
	var resultErr error
	runSSHCommandTesting := func(host string, command []string, stdin string) (string, string, error) {
		c.Assert(host, tc.Equals, "hostname")
		c.Assert(command, tc.DeepEquals, []string{"sudo", "/bin/bash"})
		c.Assert(stdin, tc.Equals, `
# Signal the jujud process to stop, then check it has done so.
set -x

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
    echo stopping jujud failed
    logger --id $(ps -o pid,cmd,state -p $(pgrep jujud) | awk 'NR != 1 {printf("Process %d (%s) has state %s\n", $1, $2, $3)}')
    exit 1
}
service juju-db stop && logger --id stopped juju-db
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
		err := s.env.DestroyController(c.Context(), "controller-uuid")
		if t.match == "" {
			c.Assert(err, tc.ErrorIsNil)
		} else {
			c.Assert(err, tc.ErrorMatches, t.match)
		}
	}
}

func (s *environSuite) TestConstraintsValidator(c *tc.C) {
	s.PatchValue(&sshprovisioner.DetectBaseAndHardwareCharacteristics,
		func(string, string) (instance.HardwareCharacteristics, base.Base, error) {
			amd64 := "amd64"
			return instance.HardwareCharacteristics{
				Arch: &amd64,
			}, base.Base{}, nil
		},
	)

	validator, err := s.env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 instance-type=foo tags=bar cpu-power=10 cores=2 mem=1G virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unsupported, tc.SameContents, []string{"cpu-power", "instance-type", "tags", "virt-type"})
}

func (s *environSuite) TestConstraintsValidatorInsideController(c *tc.C) {
	// Patch os.Args so it appears that we're running in "jujud", and then
	// patch the host arch so it looks like we're running arm64.
	s.PatchValue(&os.Args, []string{"/some/where/containing/jujud", "whatever"})
	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })

	validator, err := s.env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	cons := constraints.MustParse("arch=arm64")
	_, err = validator.Validate(cons)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *environSuite) TestPrecheck(c *tc.C) {
	// Patch os.Args so it appears that we're running in "jujud", and then
	// patch the host arch so it looks like we're running amd64.
	s.PatchValue(&os.Args, []string{"/some/where/containing/jujud", "whatever"})
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	constraint := constraints.MustParse("arch=amd64")

	// Prechecks with an explicit placement should fail
	err := s.env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Placement:   "42",
		Constraints: constraint,
	})
	c.Assert(err, tc.Not(tc.IsNil))
	c.Assert(err.Error(), tc.Equals, `use "juju add-machine ssh:[user@]<host>" to provision machines`)

	// Prechecks with no placement should work if the constraints match
	err = s.env.PrecheckInstance(c.Context(), environs.PrecheckInstanceParams{
		Constraints: constraint,
	})
	c.Assert(err, tc.ErrorIsNil)
}

type controllerInstancesSuite struct {
	baseEnvironSuite
}

func TestControllerInstancesSuite(t *stdtesting.T) {
	tc.Run(t, &controllerInstancesSuite{})
}
func (s *controllerInstancesSuite) TestControllerInstances(c *tc.C) {
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
		instances, err := s.env.ControllerInstances(c.Context(), "not-used")
		if test.expectedErr == "" {
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(instances, tc.DeepEquals, []instance.Id{BootstrapInstanceId})
		} else {
			c.Assert(err, tc.ErrorMatches, test.expectedErr)
			c.Assert(instances, tc.HasLen, 0)
		}
	}
}

func (s *controllerInstancesSuite) TestControllerInstancesStderr(c *tc.C) {
	// Stderr should not affect the behaviour of ControllerInstances.
	testhelpers.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 0")
	_, err := s.env.ControllerInstances(c.Context(), "not-used")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *controllerInstancesSuite) TestControllerInstancesError(c *tc.C) {
	// If the ssh execution fails, its stderr will be captured in the error message.
	testhelpers.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 1")
	_, err := s.env.ControllerInstances(c.Context(), "not-used")
	c.Assert(err, tc.ErrorMatches, "abc: .*")
}

func (s *controllerInstancesSuite) TestControllerInstancesInternal(c *tc.C) {
	// Patch os.Args so it appears that we're running in "jujud".
	s.PatchValue(&os.Args, []string{"/some/where/containing/jujud", "whatever"})
	// Patch the ssh executable so that it would cause an error if we
	// were to call it.
	testhelpers.PatchExecutable(c, s, "ssh", "#!/bin/sh\nhead -n1 > /dev/null; echo abc >&2; exit 1")
	instances, err := s.env.ControllerInstances(c.Context(), "not-used")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances, tc.DeepEquals, []instance.Id{BootstrapInstanceId})
}
