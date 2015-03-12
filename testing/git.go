// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"runtime"

	gc "gopkg.in/check.v1"
)

type GitSuite struct {
	BaseSuite
}

func (t *GitSuite) SetUpTest(c *gc.C) {
	t.BaseSuite.SetUpTest(c)

	t.PatchEnvironment("HOME", c.MkDir())
	t.PatchEnvironment("LC_ALL", "C")

	t.PatchEnvironment("GIT_AUTHOR_NAME", "Foo Bar")
	t.PatchEnvironment("GIT_AUTHOR_EMAIL", "foo@example.org")
	t.PatchEnvironment("GIT_COMMITTER_NAME", "Foo Bar")
	t.PatchEnvironment("GIT_COMMITTER_EMAIL", "foo@example.org")
}

func SkipIfGitNotAvailable(c *gc.C) {
	//TODO(bogdanteleaga): Make this actually check for git
	// and work on all platforms
	if runtime.GOOS == "windows" {
		c.Skip("Skipping git tests on windows")
	}
}
