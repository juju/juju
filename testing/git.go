// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"os"

	gc "launchpad.net/gocheck"
)

type GitSuite struct {
	oldValues map[string]string
}

// We ensure that Git is told about the user name and email if the setup under which the
// tests are run does not already provide that information. These git env variables are used for
// that purpose.
var gitEnvVars = []string{
	"GIT_AUTHOR_NAME",
	"GIT_AUTHOR_EMAIL",
	"GIT_COMMITTER_NAME",
	"GIT_COMMITTER_EMAIL",
}

func (t *GitSuite) SetUpTest(c *gc.C) {
	t.oldValues = make(map[string]string)
	for _, v := range gitEnvVars {
		t.oldValues[v] = os.Getenv(v)
	}
	if t.oldValues["GIT_AUTHOR_NAME"] == "" {
		os.Setenv("GIT_AUTHOR_NAME", "Foo Bar")
	}
	if t.oldValues["GIT_AUTHOR_EMAIL"] == "" {
		os.Setenv("GIT_AUTHOR_EMAIL", "foo@example.org")
	}
	os.Setenv("GIT_COMMITTER_NAME", "$GIT_AUTHOR_NAME")
	os.Setenv("GIT_COMMITTER_EMAIL", "$GIT_AUTHOR_EMAIL")
}

func (t *GitSuite) TearDownTest(c *gc.C) {
	for k, v := range t.oldValues {
		os.Setenv(k, v)
	}
}
