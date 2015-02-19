// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner_test

import (
	"path/filepath"
	"runtime"

	envtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter/runner"
)

type WindowsHookSuite struct{}

var _ = gc.Suite(&WindowsHookSuite{})

func (s *WindowsHookSuite) TestHookCommandPowerShellScript(c *gc.C) {
	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)
	defer restorer()

	hookname := "powerShellScript.ps1"
	expected := []string{
		"powershell.exe",
		"-NonInteractive",
		"-ExecutionPolicy",
		"RemoteSigned",
		"-File",
		hookname,
	}

	c.Assert(runner.HookCommand(hookname), gc.DeepEquals, expected)
}

func (s *WindowsHookSuite) TestHookCommandNotPowerShellScripts(c *gc.C) {
	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)
	defer restorer()

	cmdhook := "somehook.cmd"
	c.Assert(runner.HookCommand(cmdhook), gc.DeepEquals, []string{cmdhook})

	bathook := "somehook.bat"
	c.Assert(runner.HookCommand(bathook), gc.DeepEquals, []string{bathook})
}

func (s *WindowsHookSuite) TestSearchHookUbuntu(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Cannot search for executables without extension on windows")
	}
	restorer := envtesting.PatchValue(&version.Current.OS, version.Ubuntu)
	defer restorer()

	charmDir := c.MkDir()
	makeCharm(c, hookSpec{
		dir:  "hooks",
		name: "something-happened",
		perm: 0755,
	}, charmDir)

	expected, err := runner.LookPath(filepath.Join(charmDir, "hooks", "something-happened"))
	c.Assert(err, jc.ErrorIsNil)
	obtained, err := runner.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.Equals, expected)
}

func (s *WindowsHookSuite) TestSearchHookWindows(c *gc.C) {
	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)
	defer restorer()

	charmDir := c.MkDir()
	makeCharm(c, hookSpec{
		dir:  "hooks",
		name: "something-happened.ps1",
		perm: 0755,
	}, charmDir)

	obtained, err := runner.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.Equals, filepath.Join(charmDir, "hooks", "something-happened.ps1"))
}

func (s *WindowsHookSuite) TestSearchHookWindowsError(c *gc.C) {
	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)
	defer restorer()

	charmDir := c.MkDir()
	makeCharm(c, hookSpec{
		dir:  "hooks",
		name: "something-happened.linux",
		perm: 0755,
	}, charmDir)

	obtained, err := runner.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
	c.Assert(err.Error(), gc.Equals, filepath.FromSlash("hooks/something-happened does not exist"))
	c.Assert(obtained, gc.Equals, "")
}
