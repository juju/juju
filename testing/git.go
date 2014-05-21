// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "launchpad.net/gocheck"
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
