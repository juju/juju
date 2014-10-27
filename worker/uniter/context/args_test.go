// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"path/filepath"

	envtesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter/context"
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

	c.Assert(context.HookCommand(hookname), gc.DeepEquals, expected)
}

func (s *WindowsHookSuite) TestHookCommandNotPowerShellScripts(c *gc.C) {
	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)
	defer restorer()

	cmdhook := "somehook.cmd"
	c.Assert(context.HookCommand(cmdhook), gc.DeepEquals, []string{cmdhook})

	bathook := "somehook.bat"
	c.Assert(context.HookCommand(bathook), gc.DeepEquals, []string{bathook})
}

func (s *WindowsHookSuite) TestSearchHookUbuntu(c *gc.C) {
	restorer := envtesting.PatchValue(&version.Current.OS, version.Ubuntu)
	defer restorer()

	charmDir := c.MkDir()
	makeCharm(c, hookSpec{
		name: "something-happened",
		perm: 0755,
	}, charmDir)

	expected, err := context.LookPath(filepath.Join(charmDir, "hooks", "something-happened"))
	c.Assert(err, gc.IsNil)
	obtained, err := context.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.Equals, expected)
}

func (s *WindowsHookSuite) TestSearchHookWindows(c *gc.C) {
	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)
	defer restorer()

	charmDir := c.MkDir()
	makeCharm(c, hookSpec{
		name: "something-happened.ps1",
		perm: 0755,
	}, charmDir)

	obtained, err := context.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
	c.Assert(err, gc.IsNil)
	c.Assert(obtained, gc.Equals, filepath.Join(charmDir, "hooks", "something-happened.ps1"))
}

func (s *WindowsHookSuite) TestSearchHookWindowsError(c *gc.C) {
	restorer := envtesting.PatchValue(&version.Current.OS, version.Windows)
	defer restorer()

	charmDir := c.MkDir()
	makeCharm(c, hookSpec{
		name: "something-happened.linux",
		perm: 0755,
	}, charmDir)

	obtained, err := context.SearchHook(charmDir, filepath.Join("hooks", "something-happened"))
	c.Assert(err, gc.ErrorMatches, "hooks/something-happened does not exist")
	c.Assert(obtained, gc.Equals, "")
}
