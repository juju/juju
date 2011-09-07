package formula_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/ensemble/go/formula"
	"path/filepath"
)

func repoDir(name string) (path string) {
	return filepath.Join("testrepo", name)
}


func (s *S) TestReadDir(c *C) {
	path := repoDir("dummy")

	dir, err := formula.ReadDir(path)
	c.Assert(err, IsNil)
	c.Assert(dir.Path(), Equals, path)
	c.Assert(dir.Meta().Name, Equals, "dummy")
	c.Assert(dir.Config().Options["title"].Default, Equals, "My Title")
	c.Assert(dir.IsExpanded(), Equals, true)
}
