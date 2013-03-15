package testing

import (
	. "launchpad.net/gocheck"
	"os"
)

type GitSuite struct {
	oldValues map[string]string
}

func (t *GitSuite) SetUpSuite(c *C) {}

func (t *GitSuite) TearDownSuite(c *C) {}

func (t *GitSuite) SetUpTest(c *C) {
	// We ensure that Git is told about the user name and email if the setup under which the
	// tests are run does not already provide that information.
	t.oldValues = make(map[string]string)
	t.oldValues["GIT_AUTHOR_NAME"] = os.Getenv("GIT_AUTHOR_NAME")
	t.oldValues["GIT_AUTHOR_EMAIL"] = os.Getenv("GIT_AUTHOR_EMAIL")
	t.oldValues["GIT_COMMITTER_NAME"] = os.Getenv("GIT_COMMITTER_NAME")
	t.oldValues["GIT_COMMITTER_EMAIL"] = os.Getenv("GIT_COMMITTER_EMAIL")

	if t.oldValues["GIT_AUTHOR_NAME"] == "" {
		os.Setenv("GIT_AUTHOR_NAME", "Foo Bar")
	}
	if t.oldValues["GIT_AUTHOR_EMAIL"] == "" {
		os.Setenv("GIT_AUTHOR_EMAIL", "foo@example.org")
	}
	os.Setenv("GIT_COMMITTER_NAME", "$GIT_AUTHOR_NAME")
	os.Setenv("GIT_COMMITTER_EMAIL", "$GIT_AUTHOR_EMAIL")
}

func (t *GitSuite) TearDownTest(c *C) {
	for k, v := range t.oldValues {
		os.Setenv(k, v)
	}
}
