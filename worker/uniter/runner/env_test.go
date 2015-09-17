// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"os"
	"strings"

	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner"
)

type MergeEnvSuite struct {
	envtesting.IsolationSuite
}

var _ = gc.Suite(&MergeEnvSuite{})

func (e *MergeEnvSuite) TestMergeEnviron(c *gc.C) {
	// environment does not get fully cleared on Windows
	// when using testing.IsolationSuite
	origEnv := os.Environ()
	extraExpected := []string{
		"DUMMYVAR=foo",
		"DUMMYVAR2=bar",
		"NEWVAR=ImNew",
	}
	expectEnv := make([]string, 0, len(origEnv)+len(extraExpected))

	// os.Environ prepends some garbage on Windows that we need to strip out.
	// All the garbage starts and ends with = (for example "=C:=").
	for _, v := range origEnv {
		if !(strings.HasPrefix(v, "=") && strings.HasSuffix(v, "=")) {
			expectEnv = append(expectEnv, v)
		}
	}
	expectEnv = append(expectEnv, extraExpected...)
	os.Setenv("DUMMYVAR2", "ChangeMe")
	os.Setenv("DUMMYVAR", "foo")

	newEnv := make([]string, 0, len(expectEnv))
	for _, v := range runner.MergeWindowsEnvironment([]string{"dummyvar2=bar", "NEWVAR=ImNew"}, os.Environ()) {
		if !(strings.HasPrefix(v, "=") && strings.HasSuffix(v, "=")) {
			newEnv = append(newEnv, v)
		}
	}
	c.Assert(expectEnv, jc.SameContents, newEnv)
}

func (s *MergeEnvSuite) TestMergeEnvWin(c *gc.C) {
	initial := []string{"a=foo", "b=bar", "foo=val"}
	newValues := []string{"a=baz", "c=omg", "FOO=val2", "d=another"}

	created := runner.MergeWindowsEnvironment(newValues, initial)
	expected := []string{"a=baz", "b=bar", "c=omg", "foo=val2", "d=another"}
	c.Check(created, jc.SameContents, expected)
}
