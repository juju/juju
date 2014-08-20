// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups/config"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&pathsSuite{})

type pathsSuite struct {
	testing.BaseSuite
}

func (s *sourcesSuite) TestPathsNewPathsOkay(c *gc.C) {
	paths := config.NewPaths("a", "b", "c", "d", "e")
	root, data, startup, loggingConf, logs, ssh := config.ExposePaths(paths)

	c.Check(root, gc.Equals, "")
	c.Check(data, gc.Equals, "a")
	c.Check(startup, gc.Equals, "b")
	c.Check(loggingConf, gc.Equals, "c")
	c.Check(logs, gc.Equals, "d")
	c.Check(ssh, gc.Equals, "e")
}

func (s *sourcesSuite) TestPathsReRootOkay(c *gc.C) {
	original := config.NewPaths("a", "b", "c", "d", "e")
	paths := config.ReRoot(original, "/some_root")
	root, data, startup, loggingConf, logs, ssh := config.ExposePaths(paths)

	c.Check(root, gc.Equals, "/some_root")
	c.Check(data, gc.Equals, "a")
	c.Check(startup, gc.Equals, "b")
	c.Check(loggingConf, gc.Equals, "c")
	c.Check(logs, gc.Equals, "d")
	c.Check(ssh, gc.Equals, "e")
}

func (s *sourcesSuite) TestPathsFSReRoot(c *gc.C) {
	original := config.NewPaths("a", "b", "c", "d", "e")
	paths := config.ReRoot(original, "/")
	root, data, startup, loggingConf, logs, ssh := config.ExposePaths(paths)

	c.Check(root, gc.Equals, "/")
	c.Check(data, gc.Equals, "a")
	c.Check(startup, gc.Equals, "b")
	c.Check(loggingConf, gc.Equals, "c")
	c.Check(logs, gc.Equals, "d")
	c.Check(ssh, gc.Equals, "e")
}

func (s *sourcesSuite) TestPathsRelativeReRoot(c *gc.C) {
	original := config.NewPaths("a", "b", "c", "d", "e")
	paths := config.ReRoot(original, "some_root")
	root, data, startup, loggingConf, logs, ssh := config.ExposePaths(paths)

	c.Check(root, gc.Equals, "some_root")
	c.Check(data, gc.Equals, "a")
	c.Check(startup, gc.Equals, "b")
	c.Check(loggingConf, gc.Equals, "c")
	c.Check(logs, gc.Equals, "d")
	c.Check(ssh, gc.Equals, "e")
}

func (s *sourcesSuite) TestPathsDefaultPaths(c *gc.C) {
	paths := config.DefaultPaths
	root, data, startup, loggingConf, logs, ssh := config.ExposePaths(&paths)

	c.Check(root, gc.Equals, "")
	c.Check(data, gc.Equals, "/var/lib/juju")
	c.Check(startup, gc.Equals, "/etc/init")
	c.Check(loggingConf, gc.Equals, "/etc/rsyslog.d")
	c.Check(logs, gc.Equals, "/var/log/juju")
	c.Check(ssh, gc.Equals, "/home/ubuntu/.ssh")
}

func (s *sourcesSuite) TestPathsResolveOkay(c *gc.C) {
	paths := config.NewPaths("a", "b", "c", "d", "e")

	path, err := config.ResolvePath(paths, "data", "spam")
	c.Check(err, gc.IsNil)
	c.Check(path, gc.Equals, "a/spam")

	path, err = config.ResolvePath(paths, "startup", "spam")
	c.Check(err, gc.IsNil)
	c.Check(path, gc.Equals, "b/spam")

	path, err = config.ResolvePath(paths, "loggingConf", "spam")
	c.Check(err, gc.IsNil)
	c.Check(path, gc.Equals, "c/spam")

	path, err = config.ResolvePath(paths, "logs", "spam")
	c.Check(err, gc.IsNil)
	c.Check(path, gc.Equals, "d/spam")

	path, err = config.ResolvePath(paths, "ssh", "spam")
	c.Check(err, gc.IsNil)
	c.Check(path, gc.Equals, "e/spam")
}

func (s *sourcesSuite) TestPathsResolveNotFound(c *gc.C) {
	paths := config.NewPaths("a", "b", "c", "d", "e")
	_, err := config.ResolvePath(paths, "bogus", "spam")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *sourcesSuite) TestPathsFindEveryOkay(c *gc.C) {
	dataDir := c.MkDir()
	path := filepath.Join(dataDir, "spam")
	file, err := os.Create(path)
	c.Assert(err, gc.IsNil)
	file.Close()
	paths := config.NewPaths(dataDir, "", "", "", "")
	filenames, err := paths.FindEvery([]string{"data", "spam"})
	c.Assert(err, gc.IsNil)

	c.Check(filenames, jc.SameContents, []string{path})
}

func (s *sourcesSuite) TestPathsFindEveryMissing(c *gc.C) {
	dataDir := c.MkDir()
	paths := config.NewPaths(dataDir, "", "", "", "")
	_, err := paths.FindEvery([]string{"data", "spam"})

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *sourcesSuite) TestPathsFindEveryBadGlob(c *gc.C) {
	paths := config.NewPaths("a", "b", "c", "d", "e")
	_, err := paths.FindEvery([]string{"data", "[]"})

	c.Check(err, gc.NotNil)
}
