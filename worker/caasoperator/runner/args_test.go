// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"path/filepath"
	"runtime"

	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/os"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/caasoperator/runner"
)

type ArgsSuite struct{}

var _ = gc.Suite(&ArgsSuite{})

func (s *ArgsSuite) TestSearchHookUbuntu(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Cannot search for executables without extension on windows")
	}
	restorer := envtesting.PatchValue(&os.HostOS, func() os.OSType { return os.Ubuntu })
	defer restorer()

	charmDir := c.MkDir()
	makeCharm(c, hookSpec{
		dir:  "hooks",
		name: "something-happened",
		perm: 0755,
	}, charmDir)

	obtained, err := runner.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.Equals, filepath.Join(charmDir, "hooks", "something-happened"))
}
